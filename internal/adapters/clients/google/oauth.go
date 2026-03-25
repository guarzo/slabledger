package google

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"regexp"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/guarzo/slabledger/internal/domain/auth"
	apperrors "github.com/guarzo/slabledger/internal/domain/errors"
	"github.com/guarzo/slabledger/internal/domain/observability"
)

// secretRedactPattern matches common secret patterns in response bodies
var secretRedactPattern = regexp.MustCompile(`(?i)(key|token|secret|password|auth|credential)[=:]["']?[^&\s"']+`)

// sanitizeResponseBody redacts potential secrets and truncates response bodies.
// Redaction is done BEFORE truncation to prevent partial secret exposure at truncation boundary.
// Used to prevent sensitive information leaking into error messages/logs.
func sanitizeResponseBody(body []byte, maxLen int) string {
	// Redact potential secrets first to prevent partial exposure at truncation boundary
	s := string(body)
	s = secretRedactPattern.ReplaceAllString(s, "$1=[REDACTED]")

	// Truncate after redaction
	if len(s) > maxLen {
		s = s[:maxLen]
	}

	return s
}

const (
	// Google OAuth endpoints
	googleAuthURL  = "https://accounts.google.com/o/oauth2/v2/auth"
	googleTokenURL = "https://oauth2.googleapis.com/token"
	googleUserURL  = "https://www.googleapis.com/oauth2/v2/userinfo"

	// Session expiry duration
	sessionExpiry = 30 * 24 * time.Hour // 30 days
)

// OAuthService implements auth.Service using Google OAuth
type OAuthService struct {
	repo         auth.Repository
	logger       observability.Logger
	clientID     string
	clientSecret string
	redirectURI  string
	scopes       []string
	httpClient   *http.Client
}

// NewOAuthService creates a new Google OAuth service
func NewOAuthService(
	repo auth.Repository,
	logger observability.Logger,
	clientID string,
	clientSecret string,
	redirectURI string,
	scopes []string,
) *OAuthService {
	return &OAuthService{
		repo:         repo,
		logger:       logger,
		clientID:     clientID,
		clientSecret: clientSecret,
		redirectURI:  redirectURI,
		scopes:       scopes,
		httpClient:   &http.Client{Timeout: 10 * time.Second},
	}
}

// Compile-time interface check
var _ auth.Service = (*OAuthService)(nil)

// GetLoginURL generates the Google OAuth login URL
func (s *OAuthService) GetLoginURL(state string) string {
	params := url.Values{}
	params.Set("client_id", s.clientID)
	params.Set("redirect_uri", s.redirectURI)
	params.Set("response_type", "code")
	params.Set("scope", strings.Join(s.scopes, " "))
	params.Set("state", state)
	params.Set("access_type", "offline") // Request refresh token
	params.Set("prompt", "consent")      // Force consent to get refresh token

	return fmt.Sprintf("%s?%s", googleAuthURL, params.Encode())
}

// ExchangeCodeForTokens exchanges an authorization code for tokens
func (s *OAuthService) ExchangeCodeForTokens(ctx context.Context, code string) (*auth.UserTokens, error) {
	data := url.Values{}
	data.Set("code", code)
	data.Set("client_id", s.clientID)
	data.Set("client_secret", s.clientSecret)
	data.Set("redirect_uri", s.redirectURI)
	data.Set("grant_type", "authorization_code")

	req, err := http.NewRequestWithContext(
		ctx,
		http.MethodPost,
		googleTokenURL,
		strings.NewReader(data.Encode()),
	)
	if err != nil {
		return nil, err
	}

	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	resp, err := s.httpClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer func() {
		if err := resp.Body.Close(); err != nil {
			s.logger.Warn(ctx, "failed to close response body", observability.Err(err))
		}
	}()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}

	if resp.StatusCode != http.StatusOK {
		return nil, apperrors.ProviderAuthFailed("Google", fmt.Errorf("oauth error: status %d, body: %s", resp.StatusCode, sanitizeResponseBody(body, 200)))
	}

	var tokenResp struct {
		AccessToken  string `json:"access_token"`
		RefreshToken string `json:"refresh_token"`
		TokenType    string `json:"token_type"`
		ExpiresIn    int    `json:"expires_in"`
		Scope        string `json:"scope"`
	}

	if err := json.Unmarshal(body, &tokenResp); err != nil {
		return nil, err
	}

	if tokenResp.AccessToken == "" {
		return nil, apperrors.ProviderAuthFailed("Google", fmt.Errorf("received empty access token (status %d, body: %s)", resp.StatusCode, sanitizeResponseBody(body, 200)))
	}

	tokens := &auth.UserTokens{
		AccessToken:  tokenResp.AccessToken,
		RefreshToken: tokenResp.RefreshToken,
		TokenType:    tokenResp.TokenType,
		ExpiresAt:    time.Now().Add(time.Duration(tokenResp.ExpiresIn) * time.Second),
		Scope:        tokenResp.Scope,
	}

	return tokens, nil
}

// GetUserInfo fetches user profile information from Google using the access token
func (s *OAuthService) GetUserInfo(ctx context.Context, accessToken string) (*auth.UserInfo, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, googleUserURL, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "Bearer "+accessToken)

	resp, err := s.httpClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer func() {
		if err := resp.Body.Close(); err != nil {
			s.logger.Warn(ctx, "failed to close response body", observability.Err(err))
		}
	}()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}

	if resp.StatusCode != http.StatusOK {
		return nil, apperrors.ProviderAuthFailed("Google", fmt.Errorf("userinfo request failed: status %d, body: %s", resp.StatusCode, sanitizeResponseBody(body, 200)))
	}

	var userInfo struct {
		ID      string `json:"id"`
		Name    string `json:"name"`
		Email   string `json:"email"`
		Picture string `json:"picture"`
	}

	if err := json.Unmarshal(body, &userInfo); err != nil {
		return nil, err
	}

	if userInfo.ID == "" || userInfo.Email == "" {
		return nil, apperrors.ProviderInvalidResponse("Google", fmt.Errorf("userinfo response missing required fields (id=%q, email=%q)", userInfo.ID, userInfo.Email))
	}

	return &auth.UserInfo{
		ProviderID: userInfo.ID,
		Name:       userInfo.Name,
		Email:      userInfo.Email,
		AvatarURL:  userInfo.Picture,
	}, nil
}

// StoreOAuthState stores a one-time OAuth state token for CSRF protection
func (s *OAuthService) StoreOAuthState(ctx context.Context, state string, expiresAt time.Time) error {
	return s.repo.StoreOAuthState(ctx, state, expiresAt)
}

// ConsumeOAuthState atomically consumes and validates a one-time OAuth state token
func (s *OAuthService) ConsumeOAuthState(ctx context.Context, state string) (bool, error) {
	return s.repo.ConsumeOAuthState(ctx, state)
}

// CreateSession creates a new user session
func (s *OAuthService) CreateSession(ctx context.Context, userID int64, userAgent, ipAddress string) (*auth.Session, error) {
	session := &auth.Session{
		ID:             uuid.New().String(),
		UserID:         userID,
		ExpiresAt:      time.Now().Add(sessionExpiry),
		CreatedAt:      time.Now(),
		LastAccessedAt: time.Now(),
		UserAgent:      userAgent,
		IPAddress:      ipAddress,
	}

	if err := s.repo.CreateSession(ctx, session); err != nil {
		return nil, err
	}

	return session, nil
}

// ValidateSession validates a session and returns the session and associated user
func (s *OAuthService) ValidateSession(ctx context.Context, sessionID string) (*auth.Session, *auth.User, error) {
	session, err := s.repo.GetSession(ctx, sessionID)
	if err != nil {
		return nil, nil, err
	}

	// Check expiry
	if session.ExpiresAt.Before(time.Now()) {
		return nil, nil, apperrors.SessionExpired()
	}

	// Get user
	user, err := s.repo.GetUserByID(ctx, session.UserID)
	if err != nil {
		return nil, nil, err
	}

	// Update last accessed, but skip the DB write if the session was
	// accessed within the last 60 seconds to reduce write pressure.
	if time.Since(session.LastAccessedAt) > 60*time.Second {
		if err := s.repo.UpdateSessionAccess(ctx, sessionID); err != nil {
			s.logger.Warn(ctx, "failed to update session access time",
				observability.Int64("user_id", session.UserID),
				observability.Err(err))
		}
	}

	return session, user, nil
}

// DeleteSession deletes a session (logout)
func (s *OAuthService) DeleteSession(ctx context.Context, sessionID string) error {
	return s.repo.DeleteSession(ctx, sessionID)
}

// CleanupExpiredSessions removes all expired sessions
func (s *OAuthService) CleanupExpiredSessions(ctx context.Context) (int, error) {
	count, err := s.repo.DeleteExpiredSessions(ctx)
	if err != nil {
		return 0, err
	}

	if count > 0 {
		s.logger.Info(ctx, "cleaned up expired sessions",
			observability.Int("count", count))
	}

	return count, nil
}

// GetOrCreateUser gets an existing user or creates a new one
func (s *OAuthService) GetOrCreateUser(ctx context.Context, googleID, username, email, avatarURL string) (*auth.User, error) {
	// Try to get existing user
	user, err := s.repo.GetUserByGoogleID(ctx, googleID)
	if err == nil {
		// Update last login
		if err := s.UpdateLastLogin(ctx, user.ID); err != nil {
			s.logger.Warn(ctx, "failed to update last login",
				observability.Int64("user_id", user.ID),
				observability.Err(err))
		}
		return user, nil
	}

	// Only create a new user if the error indicates "not found"
	// For any other error (transient DB/connection errors), propagate it
	if !errors.Is(err, auth.ErrUserNotFound) {
		return nil, apperrors.StorageError("get user by google id", err)
	}

	// User not found, create new one
	user, err = s.repo.CreateUser(ctx, googleID, username, email, avatarURL)
	if err != nil {
		return nil, err
	}

	s.logger.Info(ctx, "created new user",
		observability.Int64("user_id", user.ID))

	return user, nil
}

// GetUserByID retrieves a user by ID
func (s *OAuthService) GetUserByID(ctx context.Context, userID int64) (*auth.User, error) {
	return s.repo.GetUserByID(ctx, userID)
}

// UpdateLastLogin updates the user's last login timestamp
func (s *OAuthService) UpdateLastLogin(ctx context.Context, userID int64) error {
	user, err := s.repo.GetUserByID(ctx, userID)
	if err != nil {
		return err
	}

	now := time.Now()
	user.LastLoginAt = &now

	return s.repo.UpdateUser(ctx, user)
}

// StoreTokens stores OAuth tokens for a user and session (multi-device support)
func (s *OAuthService) StoreTokens(ctx context.Context, userID int64, sessionID string, tokens *auth.UserTokens) error {
	return s.repo.StoreTokens(ctx, userID, sessionID, tokens)
}

// IsEmailAllowed checks if an email is in the allowlist
func (s *OAuthService) IsEmailAllowed(ctx context.Context, email string) (bool, error) {
	return s.repo.IsEmailAllowed(ctx, email)
}

// ListAllowedEmails returns all emails in the allowlist
func (s *OAuthService) ListAllowedEmails(ctx context.Context) ([]auth.AllowedEmail, error) {
	return s.repo.ListAllowedEmails(ctx)
}

// AddAllowedEmail adds an email to the allowlist
func (s *OAuthService) AddAllowedEmail(ctx context.Context, email string, addedBy int64, notes string) error {
	return s.repo.AddAllowedEmail(ctx, email, addedBy, notes)
}

// RemoveAllowedEmail removes an email from the allowlist
func (s *OAuthService) RemoveAllowedEmail(ctx context.Context, email string) error {
	return s.repo.RemoveAllowedEmail(ctx, email)
}

// ListUsers returns all registered users
func (s *OAuthService) ListUsers(ctx context.Context) ([]auth.User, error) {
	return s.repo.ListUsers(ctx)
}

// SetUserAdmin sets the admin flag on a user
func (s *OAuthService) SetUserAdmin(ctx context.Context, userID int64, isAdmin bool) error {
	return s.repo.SetUserAdmin(ctx, userID, isAdmin)
}
