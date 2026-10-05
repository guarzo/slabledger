package handlers

import (
	"bytes"
	"encoding/json"
	"io"
	"strconv"
	"strings"

	"github.com/guarzo/slabledger/internal/adapters/clients/dh"
	"github.com/guarzo/slabledger/internal/domain/inventory"
)

type linkInputError struct {
	status  int
	message string
}

func (e *linkInputError) Error() string { return e.message }
func decodeLinkInput(raw []byte, v any) error {
	dec := json.NewDecoder(bytes.NewReader(raw))
	dec.DisallowUnknownFields()
	if e := dec.Decode(v); e != nil {
		return &linkInputError{400, "Invalid request body"}
	}
	var extra any
	if dec.Decode(&extra) != io.EOF {
		return &linkInputError{400, "Invalid request body"}
	}
	return nil
}
func (h *DHHandler) validateLinkPreparation(kind string, raw []byte, p *inventory.Purchase) error {
	bad := func(status int, msg string) error { return &linkInputError{status, msg} }
	if kind != "unmatch" {
		if e := inventory.AssertPSAIntakeIdentity(p); e != nil {
			return e
		}
	}
	switch kind {
	case "unmatch":
		var req unmatchDHRequest
		if e := decodeLinkInput(raw, &req); e != nil {
			return e
		}
		if p.DHPushStatus != "matched" && p.DHPushStatus != "manual" {
			return bad(409, "purchase must be matched or manual")
		}
		if !inventory.DHDependenciesPresent(h.dhUnmatcher, h.candidatesSaver, h.mappingDeleter) || (p.DHInventoryID != 0 && !inventory.DHDependenciesPresent(h.inventoryDeleter)) {
			return inventory.NewReturnConflict("coordination_unavailable", "unmatch collaborators required")
		}
	case "fix_match":
		var req fixMatchRequest
		if e := decodeLinkInput(raw, &req); e != nil {
			return e
		}
		matches := dhURLPattern.FindStringSubmatch(req.DHURL)
		if len(matches) != 2 {
			return bad(400, "invalid DH URL")
		}
		card, e := strconv.Atoi(matches[1])
		if e != nil || card <= 0 {
			return bad(400, "invalid DH card ID")
		}
		if !inventory.DHDependenciesPresent(h.inventoryPusher, h.dhFieldsUpdater, h.pushStatusUpdater, h.candidatesSaver, h.cardIDSaver) || (p.DHInventoryID != 0 && p.DHCardID != card && !inventory.DHDependenciesPresent(h.channelDelister)) {
			return inventory.NewReturnConflict("coordination_unavailable", "fix-match collaborators required")
		}
	case "retry_match":
		var req retryMatchRequest
		if e := decodeLinkInput(raw, &req); e != nil {
			return e
		}
		if p.DHPushStatus != "unmatched" {
			return bad(400, "purchase is not in unmatched status")
		}
		if !inventory.DHDependenciesPresent(h.psaImporter, h.dhFieldsUpdater, h.pushStatusUpdater, h.candidatesSaver, h.cardIDSaver) {
			return inventory.NewReturnConflict("coordination_unavailable", "retry-match collaborators required")
		}
	case "select_match":
		var req selectMatchRequest
		if e := decodeLinkInput(raw, &req); e != nil {
			return e
		}
		if strings.TrimSpace(req.PurchaseID) == "" || req.DHCardID <= 0 {
			return bad(400, "purchaseId and positive dhCardId are required")
		}
		if p.DHInventoryID != 0 {
			return nil
		}
		if !inventory.DHDependenciesPresent(h.inventoryPusher, h.dhFieldsUpdater, h.pushStatusUpdater, h.candidatesSaver, h.cardIDSaver) {
			return inventory.NewReturnConflict("coordination_unavailable", "select-match collaborators required")
		}
		if p.BuyCostCents <= 0 {
			return bad(400, "purchase has no buy cost")
		}
		if p.DHCandidatesJSON != "" {
			var candidates []dh.CertResolutionCandidate
			if json.Unmarshal([]byte(p.DHCandidatesJSON), &candidates) != nil {
				return bad(400, "malformed candidates data")
			}
			found := false
			for _, candidate := range candidates {
				if candidate.DHCardID == req.DHCardID {
					found = true
				}
			}
			if !found {
				return bad(400, "dhCardId is not among the purchase candidates")
			}
		}
	}
	return nil
}
