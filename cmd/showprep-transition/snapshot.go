package main

import (
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"os"
	"strings"
	"time"

	sp "github.com/guarzo/slabledger/internal/domain/showprep"
)

const maxSnapshotBytes = 16 << 20

func loadSnapshot(path, operation string) (sp.ListDetail, error) {
	var d sp.ListDetail
	f, err := os.Open(path)
	if err != nil {
		return d, errors.New("cannot open private snapshot")
	}
	defer func() { _ = f.Close() }()
	b, err := io.ReadAll(io.LimitReader(f, maxSnapshotBytes+1))
	if err != nil || len(b) > maxSnapshotBytes {
		return d, errors.New("snapshot unreadable or too large")
	}
	// encoding/json otherwise silently accepts duplicate keys, including inside
	// the full DTO. Reject ambiguous consent documents before typed decoding.
	tokens := json.NewDecoder(bytes.NewReader(b))
	if err := uniqueJSON(tokens, 0); err != nil {
		return d, errors.New("snapshot must be one JSON object without duplicate keys")
	}
	if _, err := tokens.Token(); err != io.EOF {
		return d, errors.New("snapshot contains trailing data")
	}
	var fields map[string]json.RawMessage
	if err := json.Unmarshal(b, &fields); err != nil {
		return d, errors.New("snapshot must be an object")
	}
	_, hasKind := fields["kind"]
	_, hasVersion := fields["version"]
	if hasKind || hasVersion {
		d, err = decodePrivateSnapshot(b)
		if err != nil {
			return d, err
		}
	} else {
		// Compatibility with full public DTO snapshots whose hidden LastCommand
		// values are all empty; the unchanged fingerprint check enforces this.
		dec := json.NewDecoder(bytes.NewReader(b))
		dec.DisallowUnknownFields()
		if err := dec.Decode(&d); err != nil {
			return d, errors.New("snapshot must be a full ListDetail with known fields")
		}
	}
	return d, validateSnapshot(d, operation)
}

func uniqueJSON(d *json.Decoder, depth int) error {
	if depth > 64 {
		return errors.New("too deep")
	}
	t, err := d.Token()
	if err != nil {
		return err
	}
	delim, ok := t.(json.Delim)
	if !ok {
		return nil
	}
	switch delim {
	case '{':
		seen := map[string]bool{}
		for d.More() {
			k, err := d.Token()
			if err != nil {
				return err
			}
			key, ok := k.(string)
			if !ok || seen[key] {
				return errors.New("duplicate key")
			}
			seen[key] = true
			if err := uniqueJSON(d, depth+1); err != nil {
				return err
			}
		}
	case '[':
		for d.More() {
			if err := uniqueJSON(d, depth+1); err != nil {
				return err
			}
		}
	default:
		return errors.New("invalid JSON")
	}
	_, err = d.Token()
	return err
}

func validateSnapshot(d sp.ListDetail, operation string) error {
	if operation != "confirm-existing" && operation != "delete-empty" {
		return errors.New("invalid operation")
	}
	if !canonicalID(d.List.ID) || strings.TrimSpace(d.List.Name) == "" || !timestamp(d.List.CreatedAt) || !timestamp(d.List.UpdatedAt) || d.List.WithdrawalGeneration < 0 || d.List.WithdrawalGeneration > sp.MaxWithdrawalGeneration || d.Items == nil {
		return errors.New("snapshot list state or cohort invalid")
	}
	members, purchases := map[string]bool{}, map[string]bool{}
	for _, i := range d.Items {
		if !canonicalID(i.ID) || i.PurchaseID == "" || members[i.ID] || purchases[i.PurchaseID] || i.Version < 1 || !timestamp(i.AddedAt) || (i.PackedAt != "" && !timestamp(i.PackedAt)) {
			return errors.New("snapshot requires unique canonical member IDs, nonempty purchase IDs and full member state")
		}
		members[i.ID], purchases[i.PurchaseID] = true, true
	}
	if d.StateFingerprint != sp.ListStateFingerprint(d.List, d.Items) {
		return errors.New("snapshot fingerprint does not match full stored state; do not edit or rebase snapshot")
	}
	if operation == "delete-empty" && len(d.Items) != 0 {
		return errors.New("delete-empty requires an empty snapshot cohort")
	}
	return nil
}
func timestamp(s string) bool {
	_, err := time.Parse(time.RFC3339Nano, s)
	return err == nil && s != ""
}
