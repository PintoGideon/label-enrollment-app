// Workflow serves the API or performs explicit schema migrations. Neither normal
// startup nor readiness checks apply migrations or seed application data.
package main

import (
	"context"
	"log/slog"
	"net"
	"os"
	"os/signal"
	"syscall"

	"github.com/PintoGideon/label-enrollment-app/apps/workflow/internal/auth"
	"github.com/PintoGideon/label-enrollment-app/apps/workflow/internal/config"
	"github.com/PintoGideon/label-enrollment-app/apps/workflow/internal/database"
	"github.com/PintoGideon/label-enrollment-app/apps/workflow/internal/httpapi"
	"github.com/PintoGideon/label-enrollment-app/apps/workflow/internal/processing"
)

func main() {
	logger := slog.New(slog.NewJSONHandler(os.Stdout, nil))
	os.Exit(run(os.Args[1:], os.Getenv, logger))
}

func run(args []string, getenv func(string) string, logger *slog.Logger) int {
	command := "serve"
	if len(args) == 1 {
		command = args[0]
	}
	if len(args) > 1 || (command != "serve" && command != "migrate") {
		logger.Error("usage: workflow [serve|migrate]")
		return 1
	}
	cfg, err := config.Load(getenv)
	if err != nil {
		logger.Error("invalid workflow configuration", "error", err.Error())
		return 1
	}
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	store, err := database.Open(ctx, cfg.DatabaseURL)
	if err != nil {
		logger.Error("could not initialize Workflow database", "error", err.Error())
		return 1
	}
	defer store.Close()
	if command == "migrate" {
		applied, err := store.Migrate(ctx)
		if err != nil {
			// Store errors are deliberately sanitized; never log a raw pgx error.
			logger.Error("Workflow migration failed", "error", err.Error())
			return 1
		}
		logger.Info("Workflow migrations complete", "applied", applied)
		return 0
	}
	verifier, err := auth.New(cfg.Auth)
	if err != nil {
		logger.Error("could not initialize Workflow authentication", "error", err.Error())
		return 1
	}
	defer verifier.Close()
	policy, err := config.LoadProcessing(cfg.ProcessingFile)
	if err != nil || (policy != nil && !cfg.Auth.Enabled()) {
		logger.Error("invalid local Workflow processing policy or disabled authentication")
		return 1
	}
	processor, err := processing.New(store, policy)
	if err != nil {
		logger.Error("could not initialize local Workflow processing")
		return 1
	}
	var processingAPI httpapi.ProcessingAPI
	if processor != nil {
		processingAPI = processor
	}
	listener, err := net.Listen("tcp", cfg.Address)
	if err != nil {
		logger.Error("could not open workflow listener")
		return 1
	}
	if processor != nil {
		workerCtx, workerCancel := context.WithCancel(ctx)
		done := make(chan struct{})
		go func() { defer close(done); processor.Work(workerCtx) }()
		defer func() { workerCancel(); <-done }()
	}
	logger.Info("workflow foundation listening", "address", listener.Addr().String(), "ready", false)
	if err := httpapi.Serve(ctx, listener, store, verifier, store, processingAPI); err != nil {
		logger.Error("workflow server stopped unexpectedly")
		return 1
	}
	logger.Info("workflow stopped")
	return 0
}
