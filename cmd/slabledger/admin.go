package main

import (
	"context"
	"fmt"
	"os"

	"github.com/guarzo/slabledger/internal/adapters/clients/tcgdex"
	"github.com/guarzo/slabledger/internal/domain/observability"
	"github.com/guarzo/slabledger/internal/platform/config"
)

// handleAdminCommand routes admin subcommands to the appropriate handler
func handleAdminCommand(args []string) error {
	if len(args) < 1 {
		return showAdminHelp()
	}

	ctx := context.Background()

	switch args[0] {
	case "cache-stats":
		return adminCacheStats(ctx)
	case "version":
		config.PrintVersion()
		return nil
	case "print-config":
		return adminPrintConfig(args[1:])
	case "analyze":
		return adminAnalyze(ctx, args[1:])
	case "help", "--help", "-h":
		return showAdminHelp()
	default:
		return fmt.Errorf("unknown admin command: %s\n\nRun 'slabledger admin help' for usage", args[0])
	}
}

func showAdminHelp() error {
	fmt.Println(`slabledger admin - Administrative and operational commands

USAGE:
    slabledger admin <command> [arguments]

COMMANDS:
    Cache Management:
        cache-stats              Show persistent cache statistics

    Configuration:
        version                  Show version information
        print-config            Print current configuration

    AI Advisor:
        analyze <type>           Run an advisor analysis locally
                                 Types: liquidation, digest
                                 Flags: --verbose, --dry-run

    Help:
        help                    Show this help message

EXAMPLES:
    slabledger admin cache-stats
    slabledger admin print-config
    slabledger admin analyze liquidation --verbose`)
	return nil
}

func adminCacheStats(ctx context.Context) error {
	fmt.Printf("Persistent Cache Statistics\n\n")

	tcgdexProv := tcgdex.NewTCGdex(nil, observability.NewNoopLogger())

	stats, err := tcgdexProv.GetCacheStats(ctx)
	if err != nil {
		return fmt.Errorf("failed to get cache stats: %w", err)
	}

	if !stats.Enabled {
		fmt.Printf("Persistent caching is not enabled\n")
		return nil
	}

	fmt.Printf("Status:          Enabled\n")
	fmt.Printf("Total Sets:      %d\n", stats.TotalSets)
	fmt.Printf("Finalized Sets:  %d (fully cached)\n", stats.FinalizedSets)
	fmt.Printf("Discovered Sets: %d (metadata only)\n", stats.DiscoveredSets)
	fmt.Printf("Last Updated:    %v\n", stats.LastUpdated)
	fmt.Printf("Registry Version: %s\n", stats.RegistryVersion)
	cacheDir := os.Getenv("TCGDEX_CACHE_DIR")
	if cacheDir == "" {
		cacheDir = "data/cache/tcgdex-sets/"
	}
	fmt.Printf("\nCache Location:  %s\n", cacheDir)

	return nil
}

func adminPrintConfig(args []string) error {
	cfg, err := config.Load(args)
	if err != nil {
		return fmt.Errorf("configuration error: %w", err)
	}
	cfg.PrintConfig()
	return nil
}
