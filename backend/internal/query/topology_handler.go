package query

import (
	"context"
	"encoding/json"
	"net/http"
	"strconv"
)

// topologyRepoIface is the interface seam TopologyHandler depends on.
// *TopologyRepo satisfies this; tests inject a fake.
type topologyRepoIface interface {
	Topology(ctx context.Context, window string, nodeLimit int) (*TopologyResponse, error)
}

// TopologyHandler serves /api/v1/topology.
type TopologyHandler struct{ repo topologyRepoIface }

// NewTopologyHandler constructs a TopologyHandler backed by repo.
func NewTopologyHandler(repo topologyRepoIface) *TopologyHandler {
	return &TopologyHandler{repo: repo}
}

// Get handles GET /v1/topology with query parameters:
//
//	window      string  one of {15m, 1h, 6h, 24h} (default 1h)
//	node_limit  int     1..300 (default 100)
func (h *TopologyHandler) Get(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()

	window := q.Get("window")
	if window == "" {
		window = "1h"
	}
	if WindowToMinutes(window) < 0 {
		http.Error(w, "window must be one of 15m,1h,6h,24h", http.StatusBadRequest)
		return
	}

	nodeLimit := 100
	if v := q.Get("node_limit"); v != "" {
		n, err := strconv.Atoi(v)
		if err != nil || n < 1 || n > 300 {
			http.Error(w, "node_limit must be an integer in [1,300]", http.StatusBadRequest)
			return
		}
		nodeLimit = n
	}

	resp, err := h.repo.Topology(r.Context(), window, nodeLimit)
	if err != nil {
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}
	if resp.Nodes == nil {
		resp.Nodes = []TopologyNode{}
	}
	if resp.Edges == nil {
		resp.Edges = []TopologyEdge{}
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(resp)
}
