package query

import (
	"net/http"

	"github.com/go-chi/chi/v5"
	chimiddleware "github.com/go-chi/chi/v5/middleware"

	"github.com/huangbaixun/openaiops-platform/backend/internal/auth"
	"github.com/huangbaixun/openaiops-platform/backend/internal/chquery"
)

// NewRouter wires the query binary's chi router.
// /livez is auth-free (docker-compose healthcheck). All /api/v1/traces*
// routes are behind auth.Middleware(resolver) — Bearer required.
func NewRouter(resolver auth.Resolver, ch *chquery.Conn) *chi.Mux {
	r := chi.NewRouter()
	r.Use(chimiddleware.RequestID, chimiddleware.RealIP, chimiddleware.Recoverer)
	r.Use(chimiddleware.Logger)

	r.Get("/livez", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("ok"))
	})

	r.Group(func(r chi.Router) {
		r.Use(auth.Middleware(resolver))
		h := NewTracesHandler(ch)
		r.Get("/api/v1/traces", h.List)
		r.Get("/api/v1/traces/{trace_id}", h.Detail)
	})
	return r
}
