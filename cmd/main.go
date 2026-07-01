package main

import (
	"context"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"google.golang.org/grpc"

	"img-validation-service/internal/config"
	"img-validation-service/internal/controller"
	grpcserver "img-validation-service/internal/grpc"
	imgvalidationv1 "img-validation-service/internal/grpc/pb/imgvalidation/v1"
	"img-validation-service/internal/service"
	"img-validation-service/internal/validation"
)

func main() {
	cfg := config.Load()
	setupLogger(cfg.LogLevel)

	var nsfwChecker validation.NSFWChecker
	if cfg.NSFWEnabled && strings.TrimSpace(cfg.NSFWEndpoint) != "" {
		nsfwChecker = validation.NewHTTPChecker(cfg.NSFWEndpoint, cfg.NSFWScoreThreshold)
		slog.Info("nsfw checker enabled",
			"endpoint", cfg.NSFWEndpoint,
			"threshold", cfg.NSFWScoreThreshold,
		)
	} else {
		nsfwChecker = validation.NewStubChecker()
		slog.Warn("nsfw checker disabled (stub pass-all; reference_id with /reject/ fails)")
	}

	validator := validation.NewValidator(nsfwChecker, cfg.NSFWScoreThreshold, cfg.MaxImageSizeBytes)
	grpcSrv := grpcserver.NewServer(validator)

	grpcServer := grpc.NewServer()
	imgvalidationv1.RegisterImageValidationServiceServer(grpcServer, grpcSrv)

	grpcAddr := fmt.Sprintf("%s:%d", cfg.AppHost, cfg.GRPCPort)
	grpcLis, err := net.Listen("tcp", grpcAddr)
	if err != nil {
		slog.Error("grpc listen failed", "error", err)
		os.Exit(1)
	}

	monitoringService := service.NewMonitoringService(cfg)
	monitoringController := controller.NewMonitoringController(monitoringService)

	httpMux := http.NewServeMux()
	apiPrefix := fmt.Sprintf("/api/v1/%s", cfg.AppName)
	monitoringController.RegisterRoutes(httpMux, apiPrefix)

	httpAddr := fmt.Sprintf("%s:%d", cfg.AppHost, cfg.HTTPPort)
	httpServer := &http.Server{Addr: httpAddr, Handler: httpMux}

	go func() {
		slog.Info("gRPC server started", "address", grpcAddr)
		if err := grpcServer.Serve(grpcLis); err != nil {
			slog.Error("grpc server stopped", "error", err)
		}
	}()

	go func() {
		slog.Info("HTTP server started", "address", httpAddr)
		if err := httpServer.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			slog.Error("http server stopped", "error", err)
			os.Exit(1)
		}
	}()

	waitForShutdown(grpcServer, httpServer)
}

func setupLogger(level string) {
	var lvl slog.Level
	switch strings.ToLower(level) {
	case "debug":
		lvl = slog.LevelDebug
	case "warn", "warning":
		lvl = slog.LevelWarn
	case "error":
		lvl = slog.LevelError
	default:
		lvl = slog.LevelInfo
	}
	slog.SetDefault(slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{Level: lvl})))
}

func waitForShutdown(grpcServer *grpc.Server, httpServer *http.Server) {
	stop := make(chan os.Signal, 1)
	signal.Notify(stop, syscall.SIGINT, syscall.SIGTERM)
	<-stop

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	slog.Info("shutting down")
	grpcServer.GracefulStop()
	if err := httpServer.Shutdown(ctx); err != nil {
		slog.Error("http shutdown failed", "error", err)
	}
}
