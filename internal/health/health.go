package health

import (
	"encoding/json"
	"net/http"
)

// health.go — simple HTTP health check endpoint for Docker/k8s probes.

// Handler returns an http.Handler that responds with a JSON health status.
func Handler() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		json.NewEncoder(w).Encode(map[string]string{"status": "ok"})
	})
}
