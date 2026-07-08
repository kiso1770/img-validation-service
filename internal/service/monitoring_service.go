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
	if !s.cfg.ReadinessSkipNSFW && s.cfg.NSFWEnabled {
		if err := validation.PingSidecar(ctx, s.cfg.NSFWEndpoint); err != nil {
			return err
		}
	}
	if !s.cfg.ReadinessSkipFace {
		if s.cfg.FaceEnabled {
			if err := validation.PingSidecar(ctx, s.cfg.FaceEndpoint); err != nil {
				return err
			}
		}
		if s.cfg.AntiSpoofEnabled {
			if err := validation.PingSidecar(ctx, s.cfg.AntiSpoofEndpoint); err != nil {
				return err
			}
		}
	}
	return nil
}
