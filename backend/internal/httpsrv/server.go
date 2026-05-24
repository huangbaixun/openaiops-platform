package httpsrv

import (
	"net/http"

	"github.com/go-chi/chi/v5"
	chimiddleware "github.com/go-chi/chi/v5/middleware"

	"github.com/huangbaixun/openaiops-platform/backend/internal/auth"
)

func NewRouter(resolver auth.Resolver) *chi.Mux {
	r := chi.NewRouter()
	r.Use(chimiddleware.RequestID, chimiddleware.RealIP, chimiddleware.Recoverer)
	r.Use(chimiddleware.Logger)

	r.With(auth.Middleware(resolver)).Get("/healthz", Healthz)

	// Auth-free liveness for docker-compose healthcheck.
	r.Get("/livez", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("ok"))
	})

	return r
}
