package httpsrv

import (
	"database/sql"
	"net/http"

	"github.com/go-chi/chi/v5"
	chimiddleware "github.com/go-chi/chi/v5/middleware"

	"github.com/huangbaixun/openaiops-platform/backend/internal/auth"
	"github.com/huangbaixun/openaiops-platform/backend/internal/identity"
)

func NewRouter(resolver auth.Resolver, db *sql.DB) *chi.Mux {
	r := chi.NewRouter()
	r.Use(chimiddleware.RequestID, chimiddleware.RealIP, chimiddleware.Recoverer)
	r.Use(chimiddleware.Logger)

	r.With(auth.Middleware(resolver)).Get("/healthz", Healthz)

	ih := identity.NewHandler(identity.NewRepo(db))
	r.Group(func(g chi.Router) {
		g.Use(auth.Middleware(resolver))
		g.Get("/api/v1/tenants", ih.List)
		g.Post("/api/v1/tenants/switch", ih.Switch)
	})

	// Auth-free liveness for docker-compose healthcheck.
	r.Get("/livez", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("ok"))
	})

	return r
}
