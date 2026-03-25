package config

import "fmt"

// Version information (set via -ldflags during build).
// Example: go build -ldflags "-X config.Version=1.0.0 -X config.Commit=$(git rev-parse --short HEAD) -X config.BuildDate=$(date -u +%Y-%m-%dT%H:%M:%SZ)"
var (
	Version   = "dev"
	Commit    = "unknown"
	BuildDate = "unknown"
)

// PrintVersion prints version information
func PrintVersion() {
	fmt.Printf("slabledger %s\n", Version)
	fmt.Printf("  Commit:     %s\n", Commit)
	fmt.Printf("  Build Date: %s\n", BuildDate)
}

// PrintConfig prints sanitized configuration (secrets masked)
func (cfg *Config) PrintConfig() {
	fmt.Println("Configuration:")
	fmt.Println("  Mode:")
	fmt.Printf("    Web Mode:        %v\n", cfg.Mode.WebMode)
	if cfg.Mode.WebMode {
		fmt.Printf("    Web Port:        %d\n", cfg.Mode.WebPort)
		fmt.Printf("    Rate Limit:      %d req/min\n", cfg.Mode.RateLimitRequests)
		fmt.Printf("    Trust Proxy:     %v\n", cfg.Mode.TrustProxy)
	}

	fmt.Println("  Server:")
	fmt.Printf("    Listen:          %s\n", cfg.Server.ListenAddr)
	fmt.Printf("    Read Timeout:    %v\n", cfg.Server.ReadTimeout)
	fmt.Printf("    Write Timeout:   %v\n", cfg.Server.WriteTimeout)

	fmt.Println("  Logging:")
	fmt.Printf("    Level:           %s\n", cfg.Logging.Level)
	fmt.Printf("    JSON:            %v\n", cfg.Logging.JSON)
}
