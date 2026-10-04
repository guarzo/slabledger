package handlers

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"github.com/guarzo/slabledger/internal/domain/inventory"
	"github.com/guarzo/slabledger/internal/domain/observability"
	"io"
	"net/http"
)

type linkExecutionKey struct{}

func (h *DHHandler) failCoordinatedWrite(w http.ResponseWriter, ctx context.Context, id string, cause error) bool {
	if !h.mutationRequired {
		return false
	}
	if h.logger != nil {
		h.logger.Error(ctx, "DH mutation local completion failed", observability.String("purchaseID", id), observability.Err(cause))
	}
	writeError(w, 500, "DH mutation local completion failed; reload confirmed-return state")
	return true
}

// Buffered responses publish only AFTER local settlement commits. Reusing the
// existing typed handlers preserves their validation and ordinary HTTP shapes.
// A non-2xx response rolls back local writes and retains the prepared marker.
type mutationResponse struct {
	header http.Header
	status int
	body   bytes.Buffer
}

func (b *mutationResponse) Header() http.Header { return b.header }
func (b *mutationResponse) WriteHeader(code int) {
	if b.status == 0 {
		b.status = code
	}
}
func (b *mutationResponse) Write(p []byte) (int, error) {
	if b.status == 0 {
		b.status = 200
	}
	return b.body.Write(p)
}
func (b *mutationResponse) flush(w http.ResponseWriter) {
	for k, vs := range b.header {
		for _, v := range vs {
			w.Header().Add(k, v)
		}
	}
	w.WriteHeader(b.status)
	_, _ = w.Write(b.body.Bytes())
}
func linkPayload(p *inventory.Purchase, body json.RawMessage, kind string) string {
	b, _ := json.Marshal(struct {
		Purchase *inventory.Purchase
		Request  json.RawMessage
		Kind     string
		Phases   []string
	}{p, body, kind, []string{"mapping", "inventory", "tracking", "old_target_delist"}})
	return string(b)
}
func (h *DHHandler) coordinateLink(w http.ResponseWriter, r *http.Request, kind string, execute http.HandlerFunc) bool {
	if !h.mutationRequired || r.Context().Value(linkExecutionKey{}) != nil {
		return false
	}
	if requireUser(w, r) == nil {
		return true
	}
	if !inventory.DHDependenciesPresent(h.mutationCoordinator, h.mutationGuards, h.purchaseLister) {
		writeConfirmedReturnError(w, inventory.NewReturnConflict("coordination_unavailable", "link mutation coordination required"))
		return true
	}
	raw, e := io.ReadAll(http.MaxBytesReader(w, r.Body, 65536))
	if e != nil {
		writeError(w, 400, "Invalid request body")
		return true
	}
	var req struct {
		PurchaseID string `json:"purchaseId"`
	}
	if json.Unmarshal(raw, &req) != nil || req.PurchaseID == "" {
		writeError(w, 400, "purchaseId is required")
		return true
	}
	// Canonicalize independently of HTTP formatting; no client key/target authority.
	var body any
	if json.Unmarshal(raw, &body) != nil {
		writeError(w, 400, "Invalid request body")
		return true
	}
	canonical, _ := json.Marshal(body)
	buffered := &mutationResponse{header: make(http.Header)}
	errHTTP := errors.New("link execution returned an error response")
	e = h.mutationCoordinator.Run(r.Context(), req.PurchaseID, func(c context.Context) (inventory.DHMutationRequest, error) {
		p, e := h.purchaseLister.GetPurchase(c, req.PurchaseID)
		if e != nil {
			return inventory.DHMutationRequest{}, e
		}
		if e := h.mutationGuards.AssertMutationAllowed(c, p.ID); e != nil {
			return inventory.DHMutationRequest{}, e
		}
		if e := h.validateLinkPreparation(kind, raw, p); e != nil {
			return inventory.DHMutationRequest{}, e
		}
		return inventory.DHMutationRequest{Kind: "link", Phase: kind, PayloadIdentity: linkPayload(p, canonical, kind)}, nil
	}, func(c context.Context, a *inventory.DHMutationAttempt) (*inventory.DHMutationSettlement, error) {
		p, e := h.purchaseLister.GetPurchase(c, req.PurchaseID)
		if e != nil {
			return nil, e
		}
		if linkPayload(p, canonical, kind) != a.PayloadIdentity {
			return nil, inventory.NewReturnConflict("identity_conflict", "prepared link request changed")
		}
		if e := h.mutationGuards.AssertMutationAllowed(c, p.ID); e != nil {
			return nil, e
		}
		if e := h.validateLinkPreparation(kind, raw, p); e != nil {
			return nil, e
		}
		owned := r.Clone(context.WithValue(c, linkExecutionKey{}, true))
		owned.Body = io.NopCloser(bytes.NewReader(raw))
		execute(buffered, owned)
		if buffered.status < 200 || buffered.status >= 300 {
			return nil, errHTTP
		}
		return &inventory.DHMutationSettlement{Outcome: "succeeded", Receipt: "whole-link-orchestration-completed:" + kind}, nil
	})
	if e != nil {
		if errors.Is(e, errHTTP) {
			buffered.flush(w)
		} else {
			var input *linkInputError
			if errors.As(e, &input) {
				writeError(w, input.status, input.message)
			} else {
				writeConfirmedReturnError(w, e)
			}
		}
		return true
	}
	buffered.flush(w)
	return true
}
