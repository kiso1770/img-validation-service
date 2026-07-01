package controller

import (
	"context"
	"net/http"
	"time"

	"img-validation-service/internal/service"
)

// MonitoringController exposes health endpoints.
type MonitoringController struct {
	service service.MonitoringService
}

func NewMonitoringController(svc service.MonitoringService) *MonitoringController {
	return &MonitoringController{service: svc}
}

func (c *MonitoringController) LivenessProbe(w http.ResponseWriter, _ *http.Request) {
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write([]byte("alive"))
}

func (c *MonitoringController) ReadinessProbe(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithTimeout(r.Context(), 2*time.Second)
	defer cancel()

	if err := c.service.CheckReady(ctx); err != nil {
		http.Error(w, "not ready", http.StatusServiceUnavailable)
		return
	}
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write([]byte("ready"))
}

func (c *MonitoringController) RegisterRoutes(mux *http.ServeMux, prefix string) {
	mux.HandleFunc(prefix+"/healthz", c.LivenessProbe)
	mux.HandleFunc(prefix+"/ready", c.ReadinessProbe)
}
