package ingest

import (
	"net/http"

	"github.com/prometheus/client_golang/prometheus/promhttp"
)

// AdminHandler serves /livez, /healthz, and /metrics on the ingester's admin port.
//
// SECURITY: this handler has no auth. /metrics exposes the default Prometheus
// registry, including Go runtime metrics that may reveal memory + goroutine
// state to anyone who can reach the port. The caller MUST ensure the bind
// address is internal-only; in docker-compose the ingester admin port is
// not published to the host (see deploy/docker-compose.yml).
func AdminHandler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("/livez", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("ok"))
	})
	mux.HandleFunc("/healthz", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("ok"))
	})
	mux.Handle("/metrics", promhttp.Handler())
	return mux
}
