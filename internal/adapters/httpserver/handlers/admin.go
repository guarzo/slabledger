package handlers

import (
	"net/http"
	"strings"

	"github.com/guarzo/slabledger/internal/domain/auth"
	"github.com/guarzo/slabledger/internal/domain/observability"
)

// AdminHandlers handles admin-related HTTP requests
type AdminHandlers struct {
	authService auth.Service
	logger      observability.Logger
}

// NewAdminHandlers creates a new admin handlers instance
func NewAdminHandlers(authService auth.Service, logger observability.Logger) *AdminHandlers {
	return &AdminHandlers{
		authService: authService,
		logger:      logger,
	}
}

// HandleListAllowedEmails returns the email allowlist
func (h *AdminHandlers) HandleListAllowedEmails(w http.ResponseWriter, r *http.Request) {
	emails, ok := serviceCall(w, r.Context(), h.logger, "failed to list allowed emails", func() ([]auth.AllowedEmail, error) {
		return h.authService.ListAllowedEmails(r.Context())
	})
	if !ok {
		return
	}
	writeJSONList(w, http.StatusOK, emails)
}

// HandleAddAllowedEmail adds an email to the allowlist
func (h *AdminHandlers) HandleAddAllowedEmail(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	user := requireUser(w, r)
	if user == nil {
		return
	}

	var input struct {
		Email string `json:"email"`
		Notes string `json:"notes"`
	}
	if !decodeBody(w, r, &input) {
		return
	}

	input.Email = strings.TrimSpace(strings.ToLower(input.Email))
	if input.Email == "" || !strings.Contains(input.Email, "@") {
		writeError(w, http.StatusBadRequest, "Invalid email address")
		return
	}

	if !serviceCallVoid(w, ctx, h.logger, "failed to add allowed email", func() error {
		return h.authService.AddAllowedEmail(ctx, input.Email, user.ID, input.Notes)
	}) {
		return
	}

	h.logger.Info(ctx, "email added to allowlist",
		observability.String("email_domain", extractDomain(input.Email)),
		observability.Int64("added_by", user.ID))

	writeJSON(w, http.StatusCreated, map[string]string{"status": "ok"})
}

// HandleRemoveAllowedEmail removes an email from the allowlist
func (h *AdminHandlers) HandleRemoveAllowedEmail(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	// Extract email from path: /api/admin/allowlist/{email}
	email := strings.TrimPrefix(r.URL.Path, "/api/admin/allowlist/")
	email = strings.TrimSpace(strings.ToLower(email))
	if email == "" {
		writeError(w, http.StatusBadRequest, "Email required")
		return
	}

	if !serviceCallVoid(w, ctx, h.logger, "failed to remove allowed email", func() error {
		return h.authService.RemoveAllowedEmail(ctx, email)
	}) {
		return
	}

	h.logger.Info(ctx, "email removed from allowlist",
		observability.String("email_domain", extractDomain(email)))

	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

// HandleBackup previously streamed a SQLite VACUUM INTO backup. Backups are
// now handled automatically by Supabase (daily snapshots with PITR on Pro).
// The endpoint is retained to avoid breaking callers — it returns 410 Gone
// with a pointer to the Supabase dashboard. The dbPath argument is ignored.
func HandleBackup(_ string, _ observability.Logger) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			writeError(w, http.StatusMethodNotAllowed, "Method not allowed")
			return
		}
		writeError(w, http.StatusGone, "Backups are managed by Supabase; use the Supabase dashboard (Database → Backups) to download a dump.")
	}
}

// HandleListUsers returns all registered users
func (h *AdminHandlers) HandleListUsers(w http.ResponseWriter, r *http.Request) {
	users, ok := serviceCall(w, r.Context(), h.logger, "failed to list users", func() ([]auth.User, error) {
		return h.authService.ListUsers(r.Context())
	})
	if !ok {
		return
	}

	resp := make([]userResponse, len(users))
	for i, u := range users {
		resp[i] = userResponse{
			ID:          u.ID,
			Username:    u.Username,
			Email:       u.Email,
			AvatarURL:   u.AvatarURL,
			IsAdmin:     u.IsAdmin,
			LastLoginAt: u.LastLoginAt,
		}
	}
	writeJSONList(w, http.StatusOK, resp)
}
