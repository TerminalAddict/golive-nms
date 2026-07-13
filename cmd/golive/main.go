package main

import (
	"context"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/TerminalAddict/golive-nms/internal/app"
)

func main() {
	if len(os.Args) > 1 && os.Args[1] == "healthcheck" {
		resp, err := http.Get("http://127.0.0.1:8080/healthz")
		if err != nil || resp.StatusCode != http.StatusOK {
			os.Exit(1)
		}
		return
	}

	logger := slog.New(slog.NewJSONHandler(os.Stdout, nil))
	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	service, err := app.New(ctx, app.ConfigFromEnv(), logger)
	if err != nil {
		logger.Error("startup failed", "error", err)
		os.Exit(1)
	}
	defer service.Close()

	go service.Monitor.Run(ctx)
	go service.Events.Run(ctx)
	go service.ConfigBackup.Run(ctx)
	go service.Store.RunRetention(ctx, service.Config.RetentionDays)
	go service.Notifier.Run(ctx)
	server := &http.Server{Addr: service.Config.Listen, Handler: service.Handler(), ReadHeaderTimeout: 5 * time.Second}
	tlsConfig, err := service.CollectorTLSConfig()
	if err != nil {
		logger.Error("collector TLS failed", "error", err)
		os.Exit(1)
	}
	collector := &http.Server{Addr: service.Config.CollectorListen, Handler: service.CollectorHandler(), TLSConfig: tlsConfig, ReadHeaderTimeout: 5 * time.Second}
	go func() {
		logger.Info("GoLive NMS listening", "address", service.Config.Listen)
		if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			logger.Error("server failed", "error", err)
			stop()
		}
	}()
	go func() {
		logger.Info("GoLive collector listening", "address", service.Config.CollectorListen)
		if err := collector.ListenAndServeTLS("", ""); err != nil && err != http.ErrServerClosed {
			logger.Error("collector failed", "error", err)
			stop()
		}
	}()
	<-ctx.Done()
	shutdown, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	_ = server.Shutdown(shutdown)
	_ = collector.Shutdown(shutdown)
}
