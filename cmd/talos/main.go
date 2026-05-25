package main

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/logic-roastery/project-talos/internal/auth"
	"github.com/logic-roastery/project-talos/internal/config"
	"github.com/logic-roastery/project-talos/internal/deploy"
	"github.com/logic-roastery/project-talos/internal/github"
	"github.com/logic-roastery/project-talos/internal/proxy/traefik"
	"github.com/logic-roastery/project-talos/internal/runtime/docker"
	"github.com/logic-roastery/project-talos/internal/server"
	"github.com/logic-roastery/project-talos/internal/store"
	"github.com/logic-roastery/project-talos/web"
)

func main() {
	logger := slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelInfo}))

	cfg, err := config.Load()
	if err != nil {
		logger.Error("failed to load config", "error", err)
		os.Exit(1)
	}

	db, err := store.NewSQLiteStore(cfg.Database.Path)
	if err != nil {
		logger.Error("failed to open database", "error", err)
		os.Exit(1)
	}
	defer db.Close()

	dockerClient, err := docker.NewClient(cfg.Docker.Host, cfg.Docker.Network, logger)
	if err != nil {
		logger.Error("failed to create docker client", "error", err)
		os.Exit(1)
	}
	defer dockerClient.Close()

	proxy := traefik.NewManager(cfg.Traefik.ConfigDir, cfg.Traefik.DataDir, cfg.Docker.Network, logger)
	if err := proxy.Init(context.Background()); err != nil {
		logger.Error("failed to init traefik", "error", err)
		os.Exit(1)
	}

	authSvc := auth.NewService(db, cfg.Auth.SessionSecret, time.Duration(cfg.Auth.SessionMaxAge)*time.Second)
	engine := deploy.NewEngine(db, db, dockerClient, proxy, logger)
	webhook := github.NewWebhookHandler(cfg.GitHub.WebhookSecret)

	// Initialize GitHub App client (optional)
	var ghClient *github.AppClient
	if cfg.GitHub.AppID != 0 {
		ghClient, err = github.NewAppClient(cfg.GitHub)
		if err != nil {
			logger.Warn("github app client not initialized", "error", err)
		} else {
			logger.Info("github app configured", "app_id", cfg.GitHub.AppID)
		}
	}

	renderer, err := web.NewRenderer()
	if err != nil {
		logger.Error("failed to create renderer", "error", err)
		os.Exit(1)
	}

	srv := server.New(db, db, db, authSvc, engine, webhook, ghClient, cfg.GitHub, dockerClient, renderer, cfg.Server.Host, logger)

	addr := fmt.Sprintf("%s:%d", cfg.Server.Host, cfg.Server.Port)
	httpServer := &http.Server{
		Addr:         addr,
		Handler:      srv.Handler(),
		ReadTimeout:  15 * time.Second,
		WriteTimeout: 15 * time.Second,
		IdleTimeout:  60 * time.Second,
	}

	go func() {
		logger.Info("starting talos", "addr", addr)
		if err := httpServer.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			logger.Error("server error", "error", err)
			os.Exit(1)
		}
	}()

	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit

	logger.Info("shutting down...")
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	if err := httpServer.Shutdown(ctx); err != nil {
		logger.Error("shutdown error", "error", err)
	}
	logger.Info("talos stopped")
}
