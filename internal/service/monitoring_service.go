package service

import (
	"context"

	"img-validation-service/internal/config"
	"img-validation-service/internal/validation"
)

// MonitoringService checks dependencies for readiness probes.
type MonitoringService interface {
	CheckReady(ctx context.Context) error
}

type monitoringService struct {
	cfg *config.Config
}

func NewMonitoringService(cfg *config.Config) MonitoringService {
	return &monitoringService{cfg: cfg}
}

func (s *monitoringService) CheckReady(ctx context.Context) error {
	if s.cfg.ReadinessSkipNSFW || !s.cfg.NSFWEnabled {
		return nil
	}
	return validation.PingSidecar(ctx, s.cfg.NSFWEndpoint)
}
