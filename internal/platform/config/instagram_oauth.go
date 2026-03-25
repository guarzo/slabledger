package config

import "os"

// InstagramOAuthConfig contains Instagram OAuth configuration.
// Uses the "Instagram Login" path (graph.instagram.com).
type InstagramOAuthConfig struct {
	AppID       string
	AppSecret   string
	RedirectURI string
}

// LoadInstagramOAuthConfig loads Instagram OAuth configuration from environment variables.
func LoadInstagramOAuthConfig() *InstagramOAuthConfig {
	return &InstagramOAuthConfig{
		AppID:       os.Getenv("INSTAGRAM_APP_ID"),
		AppSecret:   os.Getenv("INSTAGRAM_APP_SECRET"),
		RedirectURI: getEnvOrDefault("INSTAGRAM_REDIRECT_URI", "http://localhost:8081/auth/instagram/callback"),
	}
}

// IsConfigured returns true if Instagram OAuth credentials are configured
// with an explicitly set redirect URI (not the localhost default).
func (c *InstagramOAuthConfig) IsConfigured() bool {
	return c.AppID != "" && c.AppSecret != "" &&
		c.RedirectURI != "" && c.RedirectURI != "http://localhost:8081/auth/instagram/callback"
}
