package handlers

import (
	"database/sql"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

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

// HandleBackup streams a consistent SQLite backup as a downloadable file.
func HandleBackup(dbPath string, logger observability.Logger) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			writeError(w, http.StatusMethodNotAllowed, "Method not allowed")
			return
		}

		// Open source database for VACUUM INTO
		srcDB, err := sql.Open("sqlite3", dbPath)
		if err != nil {
			logger.Error(r.Context(), "backup: failed to open source db", observability.Err(err))
			writeError(w, http.StatusInternalServerError, "Internal server error")
			return
		}
		defer func() {
			if cerr := srcDB.Close(); cerr != nil {
				logger.Error(r.Context(), "backup: failed to close source db", observability.Err(cerr))
			}
		}()

		tmpFile, err := os.CreateTemp("", "slabledger-backup-*.db")
		if err != nil {
			logger.Error(r.Context(), "backup: failed to create temp file", observability.Err(err))
			writeError(w, http.StatusInternalServerError, "Internal server error")
			return
		}
		tmpPath := tmpFile.Name()
		if cerr := tmpFile.Close(); cerr != nil {
			logger.Error(r.Context(), "backup: failed to close temp file", observability.Err(cerr))
		}
		defer func() {
			if rerr := os.Remove(tmpPath); rerr != nil {
				logger.Error(r.Context(), "backup: failed to remove temp file", observability.Err(rerr))
			}
		}()

		if !filepath.IsAbs(tmpPath) {
			logger.Error(r.Context(), "backup: temp path is not absolute", observability.String("path", tmpPath))
			writeError(w, http.StatusInternalServerError, "Internal server error")
			return
		}
		if strings.ContainsAny(tmpPath, "'\";\n\r") {
			logger.Error(r.Context(), "backup: temp path contains unsafe characters", observability.String("path", tmpPath))
			writeError(w, http.StatusInternalServerError, "Internal server error")
			return
		}

		// Defense in depth: escape single quotes even after validation above
		sanitizedPath := strings.ReplaceAll(tmpPath, "'", "''")
		if _, err := srcDB.ExecContext(r.Context(), fmt.Sprintf("VACUUM INTO '%s'", sanitizedPath)); err != nil {
			logger.Error(r.Context(), "backup: VACUUM INTO failed", observability.Err(err))
			writeError(w, http.StatusInternalServerError, "Internal server error")
			return
		}

		f, err := os.Open(tmpPath)
		if err != nil {
			logger.Error(r.Context(), "backup: failed to open backup file", observability.Err(err))
			writeError(w, http.StatusInternalServerError, "Internal server error")
			return
		}
		defer func() {
			if cerr := f.Close(); cerr != nil {
				logger.Error(r.Context(), "backup: failed to close backup file", observability.Err(cerr))
			}
		}()

		filename := fmt.Sprintf("slabledger-backup-%s.db", time.Now().Format("2006-01-02"))
		w.Header().Set("Content-Type", "application/octet-stream")
		w.Header().Set("Content-Disposition", fmt.Sprintf("attachment; filename=%q", filename))
		if _, err := io.Copy(w, f); err != nil {
			logger.Error(r.Context(), "backup: failed to write response", observability.Err(err))
		}
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
