package httpsrv

import (
	"encoding/json"
	"net/http"

	"github.com/huangbaixun/openaiops-platform/backend/internal/auth"
)

type healthzResp struct {
	Status     string `json:"status"`
	TenantID   string `json:"tenant_id"`
	TenantName string `json:"tenant_name"`
}

func Healthz(w http.ResponseWriter, r *http.Request) {
	tID, err := auth.TenantID(r.Context())
	if err != nil {
		http.Error(w, "no tenant", http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(healthzResp{
		Status:     "ok",
		TenantID:   tID.String(),
		TenantName: auth.TenantName(r.Context()),
	})
}
