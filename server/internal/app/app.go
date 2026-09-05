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
	activitydb "github.com/findardi/rakda/server/internal/activity/repository/sqlc"
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
	renditionWork    func(ctx context.Context)
}

func New(pool *pgxpool.Pool, otpSecret, addr, jwtSecret string, store storage.Storage, viewer contentservice.Viewer, caches contentservice.CacheDeps, mailer sender.Sender) *App {
	otpGen := otp.New(otpSecret)
	jwtGen := token.New(jwtSecret)

	limiter := ratelimit.NewMemory()

	ghCfg := config.LoadOAuth("OAUTH_GITHUB")
	ggCfg := config.LoadOAuth("OAUTH_GOOGLE")
	providers := map[string]oauth.Provider{
		"github": oauth.NewGithub(ghCfg.ClientID, ghCfg.ClientSecret, ghCfg.RedirectURL),
		"google": oauth.NewGoogle(ggCfg.ClientID, ggCfg.ClientSecret, ggCfg.RedirectURL),
	}

	webURL := config.GetEnv("WEB_URL", "http://localhost:5173")
	trashRetention := config.EnvDurationOr("TRASH_RETENTION", defaultTrashRetention)

	reaperInterval := config.EnvDurationOr("REAPER_INTERVAL", time.Hour)

	textSweepInterval := config.EnvDurationOr("TEXT_SWEEP_INTERVAL", time.Minute)

	textSweepBatch := config.EnvIntOr("TEXT_SWEEP_BATCH", 10)

	renditionWorkers := config.EnvIntOr("RENDITION_WORKERS", 1)

	renditionSweepInterval := config.EnvDurationOr("RENDITION_SWEEP_INTERVAL", 30*time.Second)

	ocrSweepInterval := config.EnvDurationOr("OCR_SWEEP_INTERVAL", time.Minute)

	ocrSweepBatch := config.EnvIntOr("OCR_SWEEP_BATCH", 10)

	bboxSweepInterval := config.EnvDurationOr("BBOX_SWEEP_INTERVAL", time.Minute)

	bboxSweepBatch := config.EnvIntOr("BBOX_SWEEP_BATCH", 10)

	pageCacheTTL := config.EnvDurationOr("PAGE_CACHE_TTL", 7*24*time.Hour)

	pageCacheSweepInterval := config.EnvDurationOr("PAGE_CACHE_SWEEP_INTERVAL", time.Hour)

	downloadStampAsyncConcurrency := config.EnvIntOr("DOWNLOAD_STAMP_ASYNC_CONCURRENCY", 2)

	downloadStampConcurrency := config.EnvIntOr("DOWNLOAD_STAMP_CONCURRENCY", 1)

	// Pengali bandwidth dan koneksi storage, bukan RAM — bentuknya mirip
	// DOWNLOAD_STAMP_CONCURRENCY tapi membatasi hal yang berbeda.
	archiveConcurrency := config.EnvIntOr("ARCHIVE_CONCURRENCY", 1)

	archiveTTL := config.EnvDurationOr("ARCHIVE_TTL", 30*24*time.Hour)

	archiveSweepInterval := config.EnvDurationOr("ARCHIVE_SWEEP_INTERVAL", time.Hour)

	downloadJobSweepInterval := config.EnvDurationOr("DOWNLOAD_JOB_SWEEP_INTERVAL", 5*time.Minute)

	shutdownTimeout := config.EnvDurationOr("SHUTDOWN_TIMEOUT", 60*time.Second)

	// Kosong = XFF tidak pernah dipercaya (perilaku aman: IP proxy, bukan IP
	// yang bisa dipalsukan). Diisi CIDR subnet docker saat stack Traefik ditulis.
	trustedProxies, err := config.GetEnvCIDRList("TRUSTED_PROXY_CIDRS", nil)
	if err != nil {
		log.Printf("invalid TRUSTED_PROXY_CIDRS, X-Forwarded-For ignored: %v", err)
		trustedProxies = nil
	}

	activitysvc := activityservice.NewActivityService(activitydb.New(pool))
	authsvc := authservice.NewAuthService(authrepo.New(pool), otpGen, jwtGen, mailer, nil)
	accessSvc := accessservice.NewAccessService(accessrepo.New(pool), mailer, authsvc, otpGen, webURL, activitysvc)
	rendition := contentservice.RenditionDeps{
		Workers:  renditionWorkers,
		Interval: renditionSweepInterval,
		Wake:     make(chan struct{}, 1),
	}
	log.Printf("viewer: rendition workers=%d sweep interval=%s", rendition.Workers, rendition.Interval)

	contentSvc := contentservice.NewContentService(contentrepo.New(pool), store, viewer, trashRetention, activitysvc, contentservice.StampDeps{Sync: downloadStampConcurrency, Async: downloadStampAsyncConcurrency}, contentservice.ArchiveDeps{
		Concurrency: archiveConcurrency,
		TTL:         archiveTTL,
	}, caches, rendition)

	// Dibangun di sini, bukan di dalam qa.NewModule, karena arsip 13-b butuh
	// eksportir Q&A sementara QAService butuh ContentService. Urutannya:
	// content (tanpa eksportir) -> qa -> instance content milik modul rute.
	qaSvc := qaservice.NewQAService(qarepo.New(pool), contentSvc, activitysvc)

	authModule := auth.NewModule(pool, otpGen, jwtGen, mailer, limiter, providers, accessSvc)
	workspaceModule := workspace.NewModule(pool, jwtGen, accessSvc, contentSvc, activitysvc, store)
	accessModule := access.NewModule(pool, jwtGen, mailer, authsvc, otpGen, webURL, activitysvc)
	invitationModule := invitation.NewModule(pool, jwtGen, activitysvc)
	contentModule := content.NewModule(pool, jwtGen, store, viewer, trashRetention, activitysvc, contentservice.StampDeps{Sync: downloadStampConcurrency, Async: downloadStampAsyncConcurrency}, contentservice.ArchiveDeps{
		Concurrency:    archiveConcurrency,
		TTL:            archiveTTL,
		ActivityExport: activitysvc,
		QAExport:       qaSvc,
	}, caches, rendition)
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
		renditionWork: func(ctx context.Context) {
			contentSvc.RunRenditionWorkers(ctx)
		},
	}
}

func (a *App) Run() error {
	bgCtx, stopBackground := context.WithCancel(context.Background())
	defer stopBackground()

	var background sync.WaitGroup

	for _, task := range []func(context.Context){
		a.reap, a.sweep, a.ocrSweep, a.bboxSweep, a.pageCacheSweep, a.archiveSweep, a.downloadJobSweep, a.renditionWork,
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
