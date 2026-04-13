package main

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/guarzo/slabledger/internal/adapters/advisortool"
	"github.com/guarzo/slabledger/internal/adapters/clients/dh"
	scoringadapter "github.com/guarzo/slabledger/internal/adapters/scoring"
	"github.com/guarzo/slabledger/internal/adapters/storage/sqlite"
	"github.com/guarzo/slabledger/internal/domain/advisor"
	"github.com/guarzo/slabledger/internal/domain/observability"
	"github.com/guarzo/slabledger/internal/platform/config"
)

// analyzeFlags holds parsed CLI flags for the analyze command.
type analyzeFlags struct {
	analysisType string
	verbose      bool
	dryRun       bool
}

func parseAnalyzeFlags(args []string) (analyzeFlags, error) {
	var f analyzeFlags

	for _, arg := range args {
		switch arg {
		case "--verbose", "-v":
			f.verbose = true
		case "--dry-run":
			f.dryRun = true
		default:
			if strings.HasPrefix(arg, "-") {
				return f, fmt.Errorf("unknown flag: %s", arg)
			}
			if f.analysisType != "" {
				return f, fmt.Errorf("unexpected argument: %s", arg)
			}
			f.analysisType = arg
		}
	}

	if f.analysisType == "" {
		return f, fmt.Errorf("missing analysis type\n\nUsage: slabledger admin analyze <type> [--verbose] [--dry-run]\n  Types: liquidation, digest")
	}

	switch f.analysisType {
	case "liquidation", "digest":
		// valid
	default:
		return f, fmt.Errorf("unknown analysis type: %s\n\nValid types: liquidation, digest", f.analysisType)
	}

	return f, nil
}

// adminAnalyze runs an advisor analysis locally with streaming output.
func adminAnalyze(ctx context.Context, args []string) error {
	flags, err := parseAnalyzeFlags(args)
	if err != nil {
		return err
	}

	// Load configuration
	cfg, err := config.Load(nil)
	if err != nil {
		return fmt.Errorf("configuration error: %w", err)
	}
	logger := initLogger(cfg.Logging.Level, cfg.Logging.JSON)

	if flags.dryRun {
		fmt.Printf("Dry run: would execute %s analysis\n", flags.analysisType)
		fmt.Println("Configuration loaded successfully")
		fmt.Printf("  Azure AI endpoint: %s\n", maskString(cfg.Adapters.AzureAIEndpoint))
		fmt.Printf("  Azure AI deployment: %s\n", cfg.Adapters.AzureAIDeployment)
		fmt.Printf("  Database path: %s\n", cfg.Database.Path)
		return nil
	}

	// Open database
	dbPath, err := resolveDatabasePath(cfg.Database.Path)
	if err != nil {
		return fmt.Errorf("resolve database path: %w", err)
	}
	if err := os.MkdirAll(filepath.Dir(dbPath), 0755); err != nil {
		return fmt.Errorf("create database directory: %w", err)
	}
	db, err := sqlite.Open(dbPath, logger)
	if err != nil {
		return fmt.Errorf("open database: %w", err)
	}
	defer func() {
		if cerr := db.Close(); cerr != nil {
			logger.Debug(ctx, "failed to close database", observability.Err(cerr))
		}
	}()

	if err := sqlite.RunMigrations(db, cfg.Database.MigrationsPath); err != nil {
		return fmt.Errorf("run migrations: %w", err)
	}

	// Initialize providers (mirrors runServer wiring order)
	cardIDMappingRepo := sqlite.NewCardIDMappingRepository(db.DB)
	intelRepo := sqlite.NewMarketIntelligenceRepository(db.DB)
	suggestionsRepo := sqlite.NewDHSuggestionsRepository(db.DB)

	// Optional DH client
	var dhClient *dh.Client
	if cfg.Adapters.DHEnterpriseKey != "" && cfg.Adapters.DHBaseURL != "" {
		dhClient = dh.NewClient(
			cfg.Adapters.DHBaseURL,
			dh.WithLogger(logger),
			dh.WithRateLimitRPS(cfg.DH.RateLimitRPS),
			dh.WithEnterpriseKey(cfg.Adapters.DHEnterpriseKey),
		)
	}

	priceProvImpl, err := initializePriceProviders(
		ctx, logger, cardIDMappingRepo,
		dhClient,
	)
	if err != nil {
		return fmt.Errorf("initialize price providers: %w", err)
	}

	campaignsInit := initializeCampaignsService(
		ctx, &cfg, logger, db, priceProvImpl, intelRepo, nil, nil,
	)
	campaignsService := campaignsInit.service
	arbSvc := campaignsInit.arbSvc
	portSvc := campaignsInit.portSvc
	tuningSvc := campaignsInit.tuningSvc

	// AI call tracking
	aiCallRepo := sqlite.NewAICallRepository(db)

	// Advisor tool options
	gapStore := sqlite.NewGapStore(db.DB)
	advisorToolOpts := []advisortool.ExecutorOption{
		advisortool.WithIntelligenceRepo(intelRepo),
		advisortool.WithSuggestionsRepo(suggestionsRepo),
		advisortool.WithGapStore(gapStore),
		advisortool.WithArbitrageService(arbSvc),
		advisortool.WithPortfolioService(portSvc),
		advisortool.WithTuningService(tuningSvc),
	}

	_, advisorSvc, _, err := initializeAdvisorService(
		ctx, &cfg, logger, db, aiCallRepo, campaignsService,
		[]scoringadapter.ProviderOption{
			scoringadapter.WithArbitrageService(arbSvc),
			scoringadapter.WithPortfolioService(portSvc),
			scoringadapter.WithTuningService(tuningSvc),
		},
		advisorToolOpts...,
	)
	if err != nil {
		return fmt.Errorf("initialize advisor service: %w", err)
	}
	if advisorSvc == nil {
		return fmt.Errorf("advisor service not available — check AZURE_AI_ENDPOINT, AZURE_AI_API_KEY, and AZURE_AI_DEPLOYMENT")
	}

	fmt.Printf("Running %s analysis...\n\n", flags.analysisType)

	callback := buildStreamCallback(flags.verbose)

	start := time.Now()
	switch flags.analysisType {
	case "liquidation":
		err = advisorSvc.AnalyzeLiquidation(ctx, callback)
	case "digest":
		err = advisorSvc.GenerateDigest(ctx, callback)
	}
	if err != nil {
		return fmt.Errorf("%s analysis failed: %w", flags.analysisType, err)
	}

	fmt.Printf("\n\n--- Completed in %s ---\n", time.Since(start).Truncate(time.Millisecond))
	return nil
}

// buildStreamCallback returns a streaming callback that prints events to stdout.
func buildStreamCallback(verbose bool) func(advisor.StreamEvent) {
	var toolCall int

	return func(evt advisor.StreamEvent) {
		switch evt.Type {
		case advisor.EventToolStart:
			if verbose {
				toolCall++
				fmt.Printf("\n[tool %d] %s ...\n", toolCall, evt.ToolName)
			}
		case advisor.EventToolResult:
			if verbose {
				content := evt.Content
				if len(content) > 200 {
					content = content[:200] + "..."
				}
				fmt.Printf("[tool %d] result (%s): %s\n", toolCall, evt.ToolName, content)
			}
		case advisor.EventDelta:
			fmt.Print(evt.Content)
		case advisor.EventDone:
			// Final newline handled in caller
		case advisor.EventError:
			fmt.Fprintf(os.Stderr, "\nError: %s\n", evt.Content)
		case advisor.EventScore:
			if verbose {
				fmt.Printf("\n[score] %s\n", evt.Content)
			}
		}
	}
}

// maskString masks a string for display, showing only the first 8 characters.
func maskString(s string) string {
	if len(s) <= 8 {
		return s
	}
	return s[:8] + "..."
}
