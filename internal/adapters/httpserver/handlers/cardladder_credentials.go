package handlers

import (
	"context"
	"net/http"

	cl "github.com/guarzo/slabledger/internal/adapters/clients/cardladder"
)

// SetConfiguredClient supplies the application-owned shared client lifecycle.
// Configure before registering routes; notification records intent, never runs work.
func (h *CardLadderHandler) SetConfiguredClient(client *cl.ConfiguredClient, notify func(context.Context) error) {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.configuredClient = client
	h.credentialsChanged = notify
}

func (h *CardLadderHandler) currentClient() *cl.Client {
	h.mu.Lock()
	configured, client := h.configuredClient, h.client
	h.mu.Unlock()
	if configured != nil {
		return configured.Current()
	}
	return client
}

func (h *CardLadderHandler) credentialNotificationFailed(w http.ResponseWriter) {
	writeJSON(w, http.StatusOK, map[string]string{
		"status": "saved", "workerNotification": "failed",
		"warning": "Credentials saved, but evidence worker activation failed. Use Retry failed in Admin after checking the worker status.",
	})
}
