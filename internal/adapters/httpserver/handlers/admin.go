package handlers

import (
	"context"
	"net/http"
	"strings"

	"github.com/guarzo/slabledger/internal/domain/auth"
	"github.com/guarzo/slabledger/internal/domain/observability"
)

// AdminDirectory is the slice of auth.Service the admin handlers actually use:
// the email allowlist and the user roster. Declared here, on the consumer side,
// so these handlers do not depend on the twenty-method auth.Service composite
// (SLA-95). The OAuth handlers in this package keep the wide seam — they use
// eleven of its methods.
type AdminDirectory interface {
	ListAllowedEmails(ctx context.Context) ([]auth.AllowedEmail, error)
	AddAllowedEmail(ctx context.Context, email string, addedBy int64, notes string) error
	RemoveAllowedEmail(ctx context.Context, email string) error
	ListUsers(ctx context.Context) ([]auth.User, error)
}

var _ AdminDirectory = auth.Service(nil)

// AdminHandlers handles admin-related HTTP requests
type AdminHandlers struct {
	authService AdminDirectory
	logger      observability.Logger
}

// NewAdminHandlers creates a new admin handlers instance
func NewAdminHandlers(authService AdminDirectory, logger observability.Logger) *AdminHandlers {
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
