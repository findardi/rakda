package app

import (
	"context"
	"errors"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/findardi/Riksa-App/server/internal/access"
	accessrepo "github.com/findardi/Riksa-App/server/internal/access/repository"
	accessservice "github.com/findardi/Riksa-App/server/internal/access/service"
	activityrepo "github.com/findardi/Riksa-App/server/internal/activity/repository"
	"github.com/findardi/Riksa-App/server/internal/auth"
	authrepo "github.com/findardi/Riksa-App/server/internal/auth/repository"
	authservice "github.com/findardi/Riksa-App/server/internal/auth/service"
	"github.com/findardi/Riksa-App/server/internal/content"
	contentrepo "github.com/findardi/Riksa-App/server/internal/content/repository"
	contentservice "github.com/findardi/Riksa-App/server/internal/content/service"

	activityservice "github.com/findardi/Riksa-App/server/internal/activity/service"

	"github.com/findardi/Riksa-App/server/internal/invitation"
	"github.com/findardi/Riksa-App/server/internal/platform/config"
	"github.com/findardi/Riksa-App/server/internal/platform/oauth"
	"github.com/findardi/Riksa-App/server/internal/platform/otp"
	"github.com/findardi/Riksa-App/server/internal/platform/ratelimit"
	"github.com/findardi/Riksa-App/server/internal/platform/response"
	"github.com/findardi/Riksa-App/server/internal/platform/sender"
	"github.com/findardi/Riksa-App/server/internal/platform/storage"
	"github.com/findardi/Riksa-App/server/internal/platform/token"
	"github.com/findardi/Riksa-App/server/internal/workspace"
	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

type App struct {
	router chi.Router
	addr   string
	reap   func(ctx context.Context)
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
	trashRetention, err := config.GetEnvDuration("TRASH_RETENTION", 15*time.Hour)
	if err != nil {
		log.Printf("invalid TRASH_RETENTION, fallback to 15h: %v", err)
		trashRetention = 15 * time.Hour
	}

	reaperInterval, err := config.GetEnvDuration("REAPER_INTERVAL", time.Hour)
	if err != nil {
		log.Printf("invalid REAPER_INTERVAL, fallback to 1h: %v", err)
		reaperInterval = time.Hour
	}

	activitysvc := activityservice.NewActivityService(activityrepo.New(pool))
	authsvc := authservice.NewAuthService(authrepo.New(pool), otpGen, jwtGen, mailer, nil)
	accessSvc := accessservice.NewAccessService(accessrepo.New(pool), mailer, authsvc, otpGen, webURL, activitysvc)
	contentSvc := contentservice.NewContentService(contentrepo.New(pool), store, viewer, trashRetention, activitysvc)

	authModule := auth.NewModule(pool, otpGen, jwtGen, mailer, limiter, providers, accessSvc)
	workspaceModule := workspace.NewModule(pool, jwtGen, accessSvc, contentSvc)
	accessModule := access.NewModule(pool, jwtGen, mailer, authsvc, otpGen, webURL, activitysvc)
	invitationModule := invitation.NewModule(pool, jwtGen, activitysvc)
	contentModule := content.NewModule(pool, jwtGen, store, viewer, trashRetention, activitysvc)

	r := chi.NewRouter()
	registerGlobalMiddleware(r)

	r.Get("/check", func(w http.ResponseWriter, r *http.Request) {
		response.Success(w, http.StatusOK, "Server Listen", nil)
	})

	authModule.RegisterRoutes(r)
	workspaceModule.RegisterRoutes(r)
	accessModule.RegisterRoutes(r)
	invitationModule.RegisterRoutes(r)
	contentModule.RegisterRoutes(r)

	return &App{
		router: r,
		addr:   addr,
		reap: func(ctx context.Context) {
			contentSvc.RunReaper(ctx, reaperInterval)
		},
	}
}

func (a *App) Run() error {
	reaperCtx, stopReaper := context.WithCancel(context.Background())
	defer stopReaper()

	go a.reap(reaperCtx)

	srv := &http.Server{
		Addr:    a.addr,
		Handler: a.router,
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

	stopReaper()

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	return srv.Shutdown(ctx)
}
