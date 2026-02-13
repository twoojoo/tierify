package main

import (
	"context"
	"fmt"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"tierify/internal/config"
	"tierify/internal/db/sqlite"
	"tierify/internal/handlers"
	"tierify/internal/limiter"
	"tierify/internal/logger"
	"tierify/internal/repositories"
	"tierify/internal/services"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
)

func main() {
	cfg, err := config.Load()
	if err != nil {
		fmt.Fprintf(os.Stderr, "failed to load config: %v\n", err)
		os.Exit(1)
	}

	log := logger.NewSlog(cfg.LogLevel, cfg.LogFormat)
	log.Info("starting tierify", "host", cfg.Host, "port", cfg.Port, "db_type", cfg.DBType)

	repos, err := newRepositories(cfg)
	if err != nil {
		log.Error("failed to initialize database", "error", err)
		os.Exit(1)
	}

	engine := limiter.NewEngine()

	tierSvc := services.NewTierService(repos, cfg, log)
	tenantSvc := services.NewTenantService(repos, cfg, log)
	usageSvc := services.NewUsageService(repos, cfg, log, engine)
	txSvc := services.NewTransactionService(repos, usageSvc, log)

	r := buildRouter(cfg, log, tierSvc, tenantSvc, usageSvc, txSvc)

	srv := &http.Server{
		Addr:         cfg.Addr(),
		Handler:      r,
		ReadTimeout:  cfg.ReadTimeout,
		WriteTimeout: cfg.WriteTimeout,
	}

	// Graceful shutdown.
	done := make(chan os.Signal, 1)
	signal.Notify(done, os.Interrupt, syscall.SIGTERM)

	go func() {
		log.Info("server listening", "addr", cfg.Addr())
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Error("server error", "error", err)
			os.Exit(1)
		}
	}()

	<-done
	log.Info("shutting down...")

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	if err := srv.Shutdown(ctx); err != nil {
		log.Error("shutdown error", "error", err)
	}

	log.Info("server stopped")
}

func buildRouter(cfg *config.Config, log logger.Logger, tierSvc services.TierService, tenantSvc services.TenantService, usageSvc services.UsageService, txSvc services.TransactionService) chi.Router {
	r := chi.NewRouter()

	r.Use(middleware.RequestID)
	r.Use(middleware.RealIP)
	r.Use(middleware.Recoverer)
	r.Use(middleware.Timeout(cfg.WriteTimeout))

	// Health check.
	r.Get("/health", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"status":"ok"}`))
	})

	tierHandler := handlers.NewTierHandler(tierSvc)
	tenantHandler := handlers.NewTenantHandler(tenantSvc)
	usageHandler := handlers.NewUsageHandler(usageSvc)
	txHandler := handlers.NewTransactionHandler(txSvc)

	r.Route("/api/v1", func(r chi.Router) {
		r.Mount("/tiers", tierHandler.Routes())
		r.Route("/tenants", func(r chi.Router) {
			r.Mount("/", tenantHandler.Routes())
			usageHandler.RegisterRoutes(r)
		})
		r.Mount("/transactions", txHandler.Routes())
	})

	return r
}

func newRepositories(cfg *config.Config) (*repositories.Repositories, error) {
	switch cfg.DBType {
	case "sqlite":
		return sqlite.NewRepositories(cfg.DBConnectionStr)
	default:
		return nil, fmt.Errorf("unsupported database type: %s (only sqlite is currently implemented)", cfg.DBType)
	}
}
