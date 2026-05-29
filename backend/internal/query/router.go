package query

import (
	"database/sql"
	"net/http"

	"github.com/go-chi/chi/v5"
	chimiddleware "github.com/go-chi/chi/v5/middleware"

	"github.com/huangbaixun/openaiops-platform/backend/internal/auth"
	"github.com/huangbaixun/openaiops-platform/backend/internal/chquery"
)

// NewRouter wires the query binary's chi router.
// /livez is auth-free (docker-compose healthcheck). All /v1/traces*,
// /v1/logs, /v1/services*, and /v1/topology routes are behind
// auth.Middleware(resolver) — Bearer required.
//
// Routes are registered without the /api prefix because Caddy strips
// /api before reverse-proxying (mirrors gateway). Direct hits on
// :8081 must use /v1/... — the public-facing /api/v1/... path is
// Caddy's responsibility.
func NewRouter(resolver auth.Resolver, ch *chquery.Conn, db *sql.DB) *chi.Mux {
	r := chi.NewRouter()
	r.Use(chimiddleware.RequestID, chimiddleware.RealIP, chimiddleware.Recoverer)
	r.Use(chimiddleware.Logger)

	r.Get("/livez", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("ok"))
	})

	r.Group(func(r chi.Router) {
		r.Use(auth.Middleware(resolver))

		th := NewTracesHandler(ch)
		r.Get("/v1/traces", th.List)
		r.Get("/v1/traces/{trace_id}", th.Detail)

		lh := NewLogsHandler(NewLogsRepo(ch))
		r.Get("/v1/logs", lh.List)

		// SLICE-3 T9: services + topology read APIs backed by topo-engine output.
		sh := NewServicesHandler(NewServicesRepo(ch))
		r.Get("/v1/services", sh.List)
		r.Get("/v1/services/{name}", sh.Detail)

		toph := NewTopologyHandler(NewTopologyRepo(ch))
		r.Get("/v1/topology", toph.Get)

		// PLATFORM-ASK-2: annotations write-back (PG-backed; see spec ADR-0003 deviation).
		ah := NewAnnotationsHandler(NewAnnotationsRepo(db))
		r.Post("/v1/annotations", ah.Create)
		r.Get("/v1/annotations", ah.List)
	})
	return r
}
