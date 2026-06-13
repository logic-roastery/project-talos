package main

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"syscall"
	"time"

	"github.com/logic-roastery/project-talos/internal/auth"
	"github.com/logic-roastery/project-talos/internal/backup"
	"github.com/logic-roastery/project-talos/internal/builder"
	"github.com/logic-roastery/project-talos/internal/config"
	"github.com/logic-roastery/project-talos/internal/crypto"
	"github.com/logic-roastery/project-talos/internal/deploy"
	"github.com/logic-roastery/project-talos/internal/github"
	"github.com/logic-roastery/project-talos/internal/proxy/traefik"
	"github.com/logic-roastery/project-talos/internal/runtime/docker"
	"github.com/logic-roastery/project-talos/internal/server"
	"github.com/logic-roastery/project-talos/internal/server/handlers"
	"github.com/logic-roastery/project-talos/internal/services"
	"github.com/logic-roastery/project-talos/internal/store"
	"github.com/logic-roastery/project-talos/web"
)

// Version is set at build time via ldflags.
var Version = "dev"

// persistEncryptionKey writes or updates TALOS_ENCRYPTION_KEY in the .env file.
func persistEncryptionKey(key string) error {
	envPath := ".env"

	data, err := os.ReadFile(envPath)
	if err != nil {
		if os.IsNotExist(err) {
			return os.WriteFile(envPath, []byte("TALOS_ENCRYPTION_KEY="+key+"\n"), 0600)
		}
		return err
	}

	lines := strings.Split(string(data), "\n")
	found := false
	for i, line := range lines {
		if strings.HasPrefix(line, "TALOS_ENCRYPTION_KEY=") {
			lines[i] = "TALOS_ENCRYPTION_KEY=" + key
			found = true
			break
		}
	}
	if !found {
		lines = append(lines, "TALOS_ENCRYPTION_KEY="+key)
	}

	return os.WriteFile(envPath, []byte(strings.Join(lines, "\n")), 0600)
}

func main() {
	// Handle --version flag
	if len(os.Args) > 1 && (os.Args[1] == "--version" || os.Args[1] == "-v") {
		fmt.Printf("talos %s\n", Version)
		os.Exit(0)
	}

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

	proxy := traefik.NewManager(
		cfg.Traefik.ConfigDir,
		cfg.Traefik.DataDir,
		cfg.Traefik.HostDataRoot,
		cfg.Docker.Network,
		cfg.Server.EdgeNetwork,
		cfg.Server.Domain,
		cfg.Server.ACMEEmail,
		cfg.Server.EdgeCertResolver,
		cfg.Server.EdgeEntrypoint,
		cfg.Server.ProxyMode,
		cfg.Server.Port,
		logger,
	)
	if err := proxy.Init(context.Background()); err != nil {
		logger.Error("failed to init traefik", "error", err)
		os.Exit(1)
	}

	if err := proxy.EnsureTraefik(context.Background(), dockerClient, cfg.Traefik.Image); err != nil {
		logger.Warn("traefik setup skipped", "error", err)
	}

	authSvc := auth.NewService(db, cfg.Auth.SessionSecret, time.Duration(cfg.Auth.SessionMaxAge)*time.Second)

	// Initialize encryption key — auto-generate if not set
	encKeyStr := cfg.EncryptionKey
	if encKeyStr == "" {
		key := crypto.GenerateKey()
		encKeyStr = crypto.EncodeKey(key)
		if err := persistEncryptionKey(encKeyStr); err != nil {
			logger.Warn("could not persist encryption key to .env", "error", err)
		} else {
			logger.Info("auto-generated encryption key and saved to .env")
		}
	}
	encKey, err := crypto.DecodeKey(encKeyStr)
	if err != nil {
		logger.Error("invalid encryption key", "error", err)
		os.Exit(1)
	}

	dataDir := filepath.Dir(cfg.Database.Path)
	provisioner := services.NewProvisioner(db, dockerClient, dataDir, encKey, logger)
	webhook := github.NewWebhookHandler(cfg.GitHub.WebhookSecret)

	// GitHub App client — initialized lazily by the handler on first use.
	// This avoids a startup race when the private key file is mounted after boot.
	var ghClient *github.AppClient

	// Initialize builder for talos_build mode (optional)
	var buildr *builder.Builder
	if ghClient != nil {
		buildr = builder.NewBuilder(ghClient, dockerClient, logger, dataDir)
	}

	engine := deploy.NewEngine(db, db, db, provisioner, dockerClient, proxy, buildr, logger)

	renderer, err := web.NewRenderer()
	if err != nil {
		logger.Error("failed to create renderer", "error", err)
		os.Exit(1)
	}

	// Backup manager
	backupMgr := backup.NewManager(db.DB(), db, dataDir, cfg.Backup.Dir, cfg.Backup.RetainCount, logger)
	backupH := handlers.NewBackupHandler(backupMgr, db)

	srv := server.New(db, db, db, db, authSvc, engine, proxy, provisioner, webhook, ghClient, cfg.GitHub, dockerClient, renderer, backupH, db, cfg.Server.Host, cfg.Server.Domain, cfg.Server.ProxyMode, cfg.Server.Port, logger)

	addr := fmt.Sprintf("%s:%d", cfg.Server.Host, cfg.Server.Port)
	httpServer := &http.Server{
		Addr:         addr,
		Handler:      srv.Handler(),
		ReadTimeout:  15 * time.Second,
		WriteTimeout: 15 * time.Second,
		IdleTimeout:  60 * time.Second,
	}

	// Start backup scheduler if configured.
	if cfg.Backup.IntervalMinutes > 0 {
		go backupMgr.StartScheduler(context.Background(), time.Duration(cfg.Backup.IntervalMinutes)*time.Minute)
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
