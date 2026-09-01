package app

import (
	"context"
	"errors"
	"log"
	"net/http"
	"os"
	"os/signal"
	"sync"
	"syscall"
	"time"

	"github.com/findardi/rakda/server/internal/access"
	accessrepo "github.com/findardi/rakda/server/internal/access/repository"
	accessservice "github.com/findardi/rakda/server/internal/access/service"
	"github.com/findardi/rakda/server/internal/activity"
	activityrepo "github.com/findardi/rakda/server/internal/activity/repository"
	activityservice "github.com/findardi/rakda/server/internal/activity/service"
	"github.com/findardi/rakda/server/internal/auth"
	authrepo "github.com/findardi/rakda/server/internal/auth/repository"
	authservice "github.com/findardi/rakda/server/internal/auth/service"
	"github.com/findardi/rakda/server/internal/content"
	contentrepo "github.com/findardi/rakda/server/internal/content/repository"
	contentservice "github.com/findardi/rakda/server/internal/content/service"
	"github.com/findardi/rakda/server/internal/invitation"
	"github.com/findardi/rakda/server/internal/platform/config"
	"github.com/findardi/rakda/server/internal/platform/oauth"
	"github.com/findardi/rakda/server/internal/platform/otp"
	"github.com/findardi/rakda/server/internal/platform/ratelimit"
	"github.com/findardi/rakda/server/internal/platform/response"
	"github.com/findardi/rakda/server/internal/platform/sender"
	"github.com/findardi/rakda/server/internal/platform/storage"
	"github.com/findardi/rakda/server/internal/platform/token"
	"github.com/findardi/rakda/server/internal/qa"
	qarepo "github.com/findardi/rakda/server/internal/qa/repository"
	qaservice "github.com/findardi/rakda/server/internal/qa/service"
	"github.com/findardi/rakda/server/internal/workspace"
	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

const defaultTrashRetention = 30 * 24 * time.Hour

type App struct {
	router           chi.Router
	addr             string
	shutdownTimeout  time.Duration
	reap             func(ctx context.Context)
	sweep            func(ctx context.Context)
	ocrSweep         func(ctx context.Context)
	bboxSweep        func(ctx context.Context)
	pageCacheSweep   func(ctx context.Context)
	archiveSweep     func(ctx context.Context)
	downloadJobSweep func(ctx context.Context)
}

func New(pool *pgxpool.Pool, otpSecret, addr, jwtSecret string, store storage.Storage, viewer contentservice.Viewer) *App {
	otpGen := otp.New(otpSecret)
	jwtGen := token.New(jwtSecret)

	mailCfg, _ := config.LoadMailConfig()
	mailer := sender.New(mailCfg)
	limiter := ratelimit.NewMemory()

	ghCfg := config.LoadOAuth("OAUTH_GITHUB")
	ggCfg := config.LoadOAuth("OAUTH_GOOGLE")
	providers := map[string]oauth.Provider{
		"github": oauth.NewGithub(ghCfg.ClientID, ghCfg.ClientSecret, ghCfg.RedirectURL),
		"google": oauth.NewGoogle(ggCfg.ClientID, ggCfg.ClientSecret, ggCfg.RedirectURL),
	}

	webURL := config.GetEnv("WEB_URL", "http://localhost:5173")
	trashRetention, err := config.GetEnvDuration("TRASH_RETENTION", defaultTrashRetention)
	if err != nil {
		log.Printf("invalid TRASH_RETENTION, fallback to %s: %v", defaultTrashRetention, err)
		trashRetention = defaultTrashRetention
	}

	reaperInterval, err := config.GetEnvDuration("REAPER_INTERVAL", time.Hour)
	if err != nil {
		log.Printf("invalid REAPER_INTERVAL, fallback to 1h: %v", err)
		reaperInterval = time.Hour
	}

	textSweepInterval, err := config.GetEnvDuration("TEXT_SWEEP_INTERVAL", time.Minute)
	if err != nil {
		log.Printf("invalid TEXT_SWEEP_INTERVAL, fallback to 1m: %v", err)
		textSweepInterval = time.Minute
	}

	textSweepBatch, err := config.GetEnvInt("TEXT_SWEEP_BATCH", 10)
	if err != nil {
		log.Printf("invalid TEXT_SWEEP_BATCH, fallback to 10: %v", err)
		textSweepBatch = 10
	}

	ocrSweepInterval, err := config.GetEnvDuration("OCR_SWEEP_INTERVAL", time.Minute)
	if err != nil {
		log.Printf("invalid OCR_SWEEP_INTERVAL, fallback to 1m: %v", err)
		ocrSweepInterval = time.Minute
	}

	ocrSweepBatch, err := config.GetEnvInt("OCR_SWEEP_BATCH", 10)
	if err != nil {
		log.Printf("invalid OCR_SWEEP_BATCH, fallback to 10: %v", err)
		ocrSweepBatch = 10
	}

	bboxSweepInterval, err := config.GetEnvDuration("BBOX_SWEEP_INTERVAL", time.Minute)
	if err != nil {
		log.Printf("invalid BBOX_SWEEP_INTERVAL, fallback to 1m: %v", err)
		bboxSweepInterval = time.Minute
	}

	bboxSweepBatch, err := config.GetEnvInt("BBOX_SWEEP_BATCH", 10)
	if err != nil {
		log.Printf("invalid BBOX_SWEEP_BATCH, fallback to 10: %v", err)
		bboxSweepBatch = 10
	}

	pageCacheTTL, err := config.GetEnvDuration("PAGE_CACHE_TTL", 7*24*time.Hour)
	if err != nil {
		log.Printf("invalid PAGE_CACHE_TTL, fallback to 168h: %v", err)
		pageCacheTTL = 7 * 24 * time.Hour
	}

	pageCacheSweepInterval, err := config.GetEnvDuration("PAGE_CACHE_SWEEP_INTERVAL", time.Hour)
	if err != nil {
		log.Printf("invalid PAGE_CACHE_SWEEP_INTERVAL, fallback to 1h: %v", err)
		pageCacheSweepInterval = time.Hour
	}

	downloadStampConcurrency, err := config.GetEnvInt("DOWNLOAD_STAMP_CONCURRENCY", 1)
	if err != nil {
		log.Printf("invalid DOWNLOAD_STAMP_CONCURRENCY, fallback to 1: %v", err)
		downloadStampConcurrency = 1
	}

	// Pengali bandwidth dan koneksi storage, bukan RAM — bentuknya mirip
	// DOWNLOAD_STAMP_CONCURRENCY tapi membatasi hal yang berbeda.
	archiveConcurrency, err := config.GetEnvInt("ARCHIVE_CONCURRENCY", 1)
	if err != nil {
		log.Printf("invalid ARCHIVE_CONCURRENCY, fallback to 1: %v", err)
		archiveConcurrency = 1
	}

	archiveTTL, err := config.GetEnvDuration("ARCHIVE_TTL", 30*24*time.Hour)
	if err != nil {
		log.Printf("invalid ARCHIVE_TTL, fallback to 720h: %v", err)
		archiveTTL = 30 * 24 * time.Hour
	}

	archiveSweepInterval, err := config.GetEnvDuration("ARCHIVE_SWEEP_INTERVAL", time.Hour)
	if err != nil {
		log.Printf("invalid ARCHIVE_SWEEP_INTERVAL, fallback to 1h: %v", err)
		archiveSweepInterval = time.Hour
	}

	downloadJobSweepInterval, err := config.GetEnvDuration("DOWNLOAD_JOB_SWEEP_INTERVAL", 5*time.Minute)
	if err != nil {
		log.Printf("invalid DOWNLOAD_JOB_SWEEP_INTERVAL, fallback to 5m: %v", err)
		downloadJobSweepInterval = 5 * time.Minute
	}

	shutdownTimeout, err := config.GetEnvDuration("SHUTDOWN_TIMEOUT", 60*time.Second)
	if err != nil {
		log.Printf("invalid SHUTDOWN_TIMEOUT, fallback to 60s: %v", err)
		shutdownTimeout = 60 * time.Second
	}

	// Kosong = XFF tidak pernah dipercaya (perilaku aman: IP proxy, bukan IP
	// yang bisa dipalsukan). Diisi CIDR subnet docker saat stack Traefik ditulis.
	trustedProxies, err := config.GetEnvCIDRList("TRUSTED_PROXY_CIDRS", nil)
	if err != nil {
		log.Printf("invalid TRUSTED_PROXY_CIDRS, X-Forwarded-For ignored: %v", err)
		trustedProxies = nil
	}

	activitysvc := activityservice.NewActivityService(activityrepo.New(pool))
	authsvc := authservice.NewAuthService(authrepo.New(pool), otpGen, jwtGen, mailer, nil)
	accessSvc := accessservice.NewAccessService(accessrepo.New(pool), mailer, authsvc, otpGen, webURL, activitysvc)
	contentSvc := contentservice.NewContentService(contentrepo.New(pool), store, viewer, trashRetention, activitysvc, downloadStampConcurrency, contentservice.ArchiveDeps{
		Concurrency: archiveConcurrency,
		TTL:         archiveTTL,
	})

	// Dibangun di sini, bukan di dalam qa.NewModule, karena arsip 13-b butuh
	// eksportir Q&A sementara QAService butuh ContentService. Urutannya:
	// content (tanpa eksportir) -> qa -> instance content milik modul rute.
	qaSvc := qaservice.NewQAService(qarepo.New(pool), contentSvc, activitysvc)

	authModule := auth.NewModule(pool, otpGen, jwtGen, mailer, limiter, providers, accessSvc)
	workspaceModule := workspace.NewModule(pool, jwtGen, accessSvc, contentSvc, activitysvc)
	accessModule := access.NewModule(pool, jwtGen, mailer, authsvc, otpGen, webURL, activitysvc)
	invitationModule := invitation.NewModule(pool, jwtGen, activitysvc)
	contentModule := content.NewModule(pool, jwtGen, store, viewer, trashRetention, activitysvc, downloadStampConcurrency, contentservice.ArchiveDeps{
		Concurrency:    archiveConcurrency,
		TTL:            archiveTTL,
		ActivityExport: activitysvc,
		QAExport:       qaSvc,
	})
	activityModule := activity.NewModule(pool, jwtGen)
	qaModule := qa.NewModule(pool, jwtGen, contentSvc, activitysvc)

	r := chi.NewRouter()
	registerGlobalMiddleware(r, trustedProxies)

	r.Get("/check", func(w http.ResponseWriter, r *http.Request) {
		response.Success(w, http.StatusOK, "Server Listen", nil)
	})

	authModule.RegisterRoutes(r)
	workspaceModule.RegisterRoutes(r)
	accessModule.RegisterRoutes(r)
	invitationModule.RegisterRoutes(r)
	contentModule.RegisterRoutes(r)
	activityModule.RegisterRoutes(r)
	qaModule.RegisterRoutes(r)

	return &App{
		router:          r,
		addr:            addr,
		shutdownTimeout: shutdownTimeout,
		reap: func(ctx context.Context) {
			contentSvc.RunReaper(ctx, reaperInterval)
		},
		sweep: func(ctx context.Context) {
			contentSvc.RunTextSweeper(ctx, textSweepInterval, textSweepBatch)
		},
		ocrSweep: func(ctx context.Context) {
			contentSvc.RunOCRSweeper(ctx, ocrSweepInterval, ocrSweepBatch)
		},
		bboxSweep: func(ctx context.Context) {
			contentSvc.RunBBoxSweeper(ctx, bboxSweepInterval, bboxSweepBatch)
		},
		pageCacheSweep: func(ctx context.Context) {
			contentSvc.RunPageCacheSweeper(ctx, pageCacheSweepInterval, pageCacheTTL)
		},
		archiveSweep: func(ctx context.Context) {
			contentSvc.RunArchiveSweeper(ctx, archiveSweepInterval, archiveTTL)
		},
		downloadJobSweep: func(ctx context.Context) {
			contentSvc.RunDownloadJobSweeper(ctx, downloadJobSweepInterval)
		},
	}
}

func (a *App) Run() error {
	bgCtx, stopBackground := context.WithCancel(context.Background())
	defer stopBackground()

	var background sync.WaitGroup

	for _, task := range []func(context.Context){
		a.reap, a.sweep, a.ocrSweep, a.bboxSweep, a.pageCacheSweep, a.archiveSweep, a.downloadJobSweep,
	} {
		background.Add(1)

		go func() {
			defer background.Done()
			task(bgCtx)
		}()
	}

	srv := &http.Server{
		Addr:    a.addr,
		Handler: a.router,
		// Timeout eksplisit (keputusan 9.5-d): tanpa WriteTimeout — timeout
		// tulis global akan memutus unduhan panjang justru pada kasus yang
		// sedang dilindungi. ReadTimeout 30s aman: body request ke Go selalu
		// kecil (unggah besar lewat presigned PUT langsung ke Minio).
		ReadHeaderTimeout: 10 * time.Second,
		ReadTimeout:       30 * time.Second,
		IdleTimeout:       120 * time.Second,
	}

	go func() {
		log.Printf("server running on %s", a.addr)
		if err := srv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			log.Fatalf("listen: %v", err)
		}
	}()

	quit := make(chan os.Signal, 1)
	signal.Notify(quit, os.Interrupt, syscall.SIGTERM)
	<-quit

	stopBackground()

	ctx, cancel := context.WithTimeout(context.Background(), a.shutdownTimeout)
	defer cancel()

	drained := make(chan struct{})

	go func() {
		background.Wait()
		close(drained)
	}()

	err := srv.Shutdown(ctx)

	select {
	case <-drained:
	case <-ctx.Done():
		log.Printf("shutdown: background tasks unfinished after %s", a.shutdownTimeout)
	}

	return err
}
