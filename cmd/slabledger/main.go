package main

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"os/signal"
	"syscall"

	"github.com/guarzo/slabledger/internal/domain/observability"
	"github.com/guarzo/slabledger/internal/platform/config"
	"github.com/guarzo/slabledger/internal/platform/telemetry"
	"github.com/joho/godotenv"

	// Concrete implementations (only imported in main for wiring - Hexagonal Architecture)
	"github.com/guarzo/slabledger/internal/adapters/clients/psaportal"
	"github.com/guarzo/slabledger/internal/adapters/storage/postgres"
)

// Compile-time guard: the Postgres snapshot store must satisfy the client's
// SnapshotStore (read) contract the provider depends on.
var _ psaportal.SnapshotStore = (*postgres.PSAPortalSnapshotStore)(nil)

// initLogger creates a new logger with the specified level and format
func initLogger(level string, jsonFormat bool) observability.Logger {
	var slogLevel slog.Level
	switch level {
	case "debug":
		slogLevel = slog.LevelDebug
	case "info":
		slogLevel = slog.LevelInfo
	case "warn":
		slogLevel = slog.LevelWarn
	case "error":
		slogLevel = slog.LevelError
	default:
		slogLevel = slog.LevelInfo
	}

	format := "text"
	if jsonFormat {
		format = "json"
	}

	return telemetry.NewSlogLogger(slogLevel, format)
}

func main() {
	if err := godotenv.Load(); err != nil {
		if !os.IsNotExist(err) {
			fmt.Fprintf(os.Stderr, "Warning: Error loading .env file: %v\n", err)
		}
	}

	var mode string
	var args []string

	if len(os.Args) == 1 {
		mode = "server"
		args = []string{}
	} else {
		firstArg := os.Args[1]

		switch firstArg {
		case "--help", "-h", "help":
			showHelp()
			os.Exit(0)
		case "--version", "-v", "version":
			config.PrintVersion()
			os.Exit(0)
		case "server":
			mode = "server"
			args = os.Args[2:]
		case "--web":
			mode = "server"
			args = os.Args[1:]
		case "admin":
			mode = "admin"
			args = os.Args[2:]
		default:
			if firstArg[0] == '-' {
				mode = "server"
				args = os.Args[1:]
			} else {
				fmt.Fprintf(os.Stderr, "Error: Unknown command '%s'\n\n", firstArg)
				showHelp()
				os.Exit(1)
			}
		}
	}

	switch mode {
	case "admin":
		if err := handleAdminCommand(args); err != nil {
			fmt.Fprintf(os.Stderr, "Admin command failed: %v\n", err)
			os.Exit(1)
		}
	case "server":
		cfg, err := config.Load(args)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Configuration error: %v\n", err)
			os.Exit(1)
		}
		logger := initLogger(cfg.Logging.Level, cfg.Logging.JSON)

		if err := runServer(&cfg, logger); err != nil {
			logger.Error(context.Background(), "Server error", observability.Err(err))
			os.Exit(1)
		}
	}
}

func showHelp() {
	fmt.Println(`slabledger - Graded Card Portfolio Tracker

USAGE:
    slabledger [command] [arguments]

COMMANDS:
    server              Start web server (default if no command specified)
    admin <command>     Administrative and operational commands
    help                Show this help message
    version             Show version information

SERVER MODE:
    slabledger                    # Start web server (default port 8081)
    slabledger server             # Explicit server mode
    slabledger server --port 9090 # Custom port

    Binds 0.0.0.0:<port>. Set HTTP_LISTEN_ADDR to override the full
    host:port, e.g. HTTP_LISTEN_ADDR=127.0.0.1:8081 for loopback only.

EXAMPLES:

Documentation: docs/USER_GUIDE.md
Web Interface: http://localhost:8081`)
}

func runServer(cfg *config.Config, logger observability.Logger) error {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	envValidation := validateEnvironmentVariables(ctx, logger, cfg)
	if len(envValidation.MissingRequired) > 0 {
		return fmt.Errorf("missing required environment variables: %v. See logs above for details", envValidation.MissingRequired)
	}

	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, os.Interrupt, syscall.SIGTERM)
	go func() {
		<-sigChan
		logger.Info(ctx, "Received interrupt signal, initiating graceful shutdown")
		cancel()
	}()

	w := &wiring{}

	// dbCleanup must be deferred before metricsCleanup. Defers run LIFO, so at
	// unwind this makes the metrics server stop first (it has its own 5s
	// shutdown timeout) and the database close last — the same order the
	// original inline runServer body registered its two defers in. Reversing
	// this would close the database before requests draining through the
	// metrics/health path finish.
	dbCleanup, metricsCleanup, err := w.setupInfrastructure(ctx, cfg, logger)
	if dbCleanup != nil {
		defer dbCleanup()
	}
	if metricsCleanup != nil {
		defer metricsCleanup()
	}
	if err != nil {
		return err
	}

	if err := w.buildRepositoriesAndAuth(ctx, cfg, logger); err != nil {
		return err
	}

	w.buildCampaignsAndIntegrations(ctx, cfg, logger)

	schedulerResult, cancelScheduler := initializeSchedulers(ctx, w.schedulerDeps(cfg, logger))

	deps, hOut := createHandlers(ctx, w.handlerInputs(cfg, logger, schedulerResult))
	serverErr := startWebServer(ctx, deps)

	shutdownGracefully(ctx, logger, cancelScheduler, schedulerResult, hOut, w.campaignsInit.service, w.campaignsInit.importService, cfg.Server.SchedulerShutdownTimeout)

	return serverErr
}
