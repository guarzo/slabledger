package main

import (
	"fmt"

	"github.com/guarzo/slabledger/internal/platform/config"
)

// handleAdminCommand routes admin subcommands to the appropriate handler
func handleAdminCommand(args []string) error {
	if len(args) < 1 {
		return showAdminHelp()
	}

	switch args[0] {
	case "version":
		config.PrintVersion()
		return nil
	case "print-config":
		return adminPrintConfig(args[1:])
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
    Configuration:
        version                  Show version information
        print-config            Print current configuration

    Help:
        help                    Show this help message

EXAMPLES:
    slabledger admin print-config`)
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
