package query

import (
	"net/http"

	"github.com/go-chi/chi/v5"
	chimiddleware "github.com/go-chi/chi/v5/middleware"

	"github.com/huangbaixun/openaiops-platform/backend/internal/auth"
	"github.com/huangbaixun/openaiops-platform/backend/internal/chquery"
)

// NewRouter wires the query binary's chi router.
// /livez is auth-free (docker-compose healthcheck). All /v1/traces*
// routes are behind auth.Middleware(resolver) — Bearer required.
//
// Routes are registered without the /api prefix because Caddy strips
// /api before reverse-proxying (mirrors gateway). Direct hits on
// :8081 must use /v1/... — the public-facing /api/v1/... path is
// Caddy's responsibility.
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
		r.Get("/v1/traces", h.List)
		r.Get("/v1/traces/{trace_id}", h.Detail)
	})
	return r
}
