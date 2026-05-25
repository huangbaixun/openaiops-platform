package ingest

import (
	"net/http"

	"github.com/prometheus/client_golang/prometheus/promhttp"
)

// AdminHandler serves /livez, /healthz, and /metrics on the ingester's admin port.
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
