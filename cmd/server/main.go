package main

import (
	"context"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"subscriptions/internal/config"
	"subscriptions/internal/db"
	"subscriptions/internal/handler"
	"subscriptions/internal/repository"
)

func main() {
	logger := slog.New(slog.NewJSONHandler(os.Stdout, nil))

	cfg, err := config.Load("config/config.yaml")
	if err != nil {
		logger.Error("failed to load config", "error", err)
		os.Exit(1)
	}

	var store repository.SubscriptionStore
	mode := strings.ToLower(cfg.Storage.Mode)

	if mode == "local" {
		repo, err := repository.NewLocalSubscriptionRepository(cfg.Storage.LocalPath)
		if err != nil {
			logger.Error("failed to init local db", "error", err)
			os.Exit(1)
		}
		store = repo
		logger.Info("using local file database", "path", cfg.Storage.LocalPath)
	} else {
		ctx := context.Background()
		pool, err := db.Connect(ctx, cfg.DatabaseURL(), logger)
		if err != nil {
			logger.Error("failed to connect db", "error", err)
			os.Exit(1)
		}
		defer pool.Close()
		store = repository.NewSubscriptionRepository(pool)
		logger.Info("using postgres database")
	}

	h := handler.New(store, logger)

	srv := &http.Server{
		Addr:         ":" + cfg.Server.Port,
		Handler:      h.Router(),
		ReadTimeout:  10 * time.Second,
		WriteTimeout: 10 * time.Second,
		IdleTimeout:  60 * time.Second,
	}

	go func() {
		logger.Info("server starting", "port", cfg.Server.Port)
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			logger.Error("server stopped", "error", err)
			os.Exit(1)
		}
	}()

	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit

	shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := srv.Shutdown(shutdownCtx); err != nil {
		logger.Error("graceful shutdown failed", "error", err)
	}
	logger.Info("server stopped")
}
