package app

import (
	"net/netip"

	platformmw "github.com/findardi/Riksa-App/server/internal/platform/middleware"
	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
)

func registerGlobalMiddleware(r chi.Router, trustedProxies []netip.Prefix) {
	r.Use(middleware.RequestID)
	r.Use(platformmw.RealIP(trustedProxies))
	r.Use(middleware.Logger)
	r.Use(middleware.Recoverer)
}
