package config

import "os"

// GoogleOAuthConfig contains Google OAuth 2.0 configuration
type GoogleOAuthConfig struct {
	ClientID     string
	ClientSecret string
	RedirectURI  string
	Scopes       []string
}

// LoadGoogleOAuthConfig loads Google OAuth configuration from environment variables
func LoadGoogleOAuthConfig() *GoogleOAuthConfig {
	return &GoogleOAuthConfig{
		ClientID:     os.Getenv("GOOGLE_CLIENT_ID"),
		ClientSecret: os.Getenv("GOOGLE_CLIENT_SECRET"),
		RedirectURI:  getEnvOrDefault("GOOGLE_REDIRECT_URI", "http://localhost:8081/auth/google/callback"),
		Scopes:       []string{"openid", "email", "profile"},
	}
}

// IsConfigured returns true if Google OAuth credentials are configured
func (c *GoogleOAuthConfig) IsConfigured() bool {
	return c.ClientID != "" && c.ClientSecret != ""
}

// getEnvOrDefault returns the environment variable value or a default if not set
func getEnvOrDefault(key, defaultValue string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return defaultValue
}
