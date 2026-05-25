package query

import (
	"net/http"

	"github.com/huangbaixun/openaiops-platform/backend/internal/chquery"
)

// TracesHandler serves /api/v1/traces* endpoints. T8 fills List; T9 fills Detail.
type TracesHandler struct{ ch *chquery.Conn }

func NewTracesHandler(ch *chquery.Conn) *TracesHandler { return &TracesHandler{ch: ch} }

func (h *TracesHandler) List(w http.ResponseWriter, _ *http.Request) {
	http.Error(w, "not implemented", http.StatusNotImplemented)
}

func (h *TracesHandler) Detail(w http.ResponseWriter, _ *http.Request) {
	http.Error(w, "not implemented", http.StatusNotImplemented)
}
