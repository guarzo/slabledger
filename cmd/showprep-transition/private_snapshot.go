package main

import (
	"bytes"
	"encoding/json"
	"errors"

	sp "github.com/guarzo/slabledger/internal/domain/showprep"
)

const privateSnapshotKind = "showprep-transition-snapshot"

// This CLI-only wire contract includes every ListStateFingerprint member field,
// without exposing LastCommand through the public API or exporting evaluations.
type privateSnapshot struct {
	Kind             string          `json:"kind"`
	Version          int             `json:"version"`
	List             sp.List         `json:"list"`
	Items            []privateMember `json:"items"`
	StateFingerprint string          `json:"stateFingerprint"`
}
type privateMember struct {
	ID                     string    `json:"id"`
	PurchaseID             string    `json:"purchaseId"`
	CardName               string    `json:"cardName"`
	CertNumber             string    `json:"certNumber"`
	Grader                 string    `json:"grader"`
	Grade                  float64   `json:"grade"`
	AddedAt                string    `json:"addedAt"`
	PackedAt               string    `json:"packedAt"`
	Version                int64     `json:"version"`
	AcknowledgedPriceCents int       `json:"acknowledgedPriceCents"`
	AcknowledgedStatus     sp.Status `json:"acknowledgedStatus"`
	LastCommand            string    `json:"lastCommand"`
}

func capturePrivateSnapshot(list sp.List, items []sp.Item) privateSnapshot {
	p := privateSnapshot{Kind: privateSnapshotKind, Version: 1, List: list, Items: make([]privateMember, 0, len(items)), StateFingerprint: sp.ListStateFingerprint(list, items)}
	for _, i := range items {
		p.Items = append(p.Items, privateMember{ID: i.ID, PurchaseID: i.PurchaseID, CardName: i.CardName, CertNumber: i.CertNumber, Grader: i.Grader, Grade: i.Grade, AddedAt: i.AddedAt, PackedAt: i.PackedAt, Version: i.Version, AcknowledgedPriceCents: i.AcknowledgedPriceCents, AcknowledgedStatus: i.AcknowledgedStatus, LastCommand: i.LastCommand})
	}
	return p
}
func (p privateSnapshot) detail() sp.ListDetail {
	d := sp.ListDetail{List: p.List, Items: make([]sp.Item, 0, len(p.Items)), StateFingerprint: p.StateFingerprint}
	for _, i := range p.Items {
		d.Items = append(d.Items, sp.Item{ID: i.ID, PurchaseID: i.PurchaseID, CardName: i.CardName, CertNumber: i.CertNumber, Grader: i.Grader, Grade: i.Grade, AddedAt: i.AddedAt, PackedAt: i.PackedAt, Version: i.Version, AcknowledgedPriceCents: i.AcknowledgedPriceCents, AcknowledgedStatus: i.AcknowledgedStatus, LastCommand: i.LastCommand})
	}
	return d
}
func decodePrivateSnapshot(b []byte) (sp.ListDetail, error) {
	var p privateSnapshot
	dec := json.NewDecoder(bytes.NewReader(b))
	dec.DisallowUnknownFields()
	if err := dec.Decode(&p); err != nil || p.Kind != privateSnapshotKind || p.Version != 1 || p.Items == nil {
		return sp.ListDetail{}, errors.New("invalid private snapshot kind/version or fields")
	}
	// Required zero-valued fields must be present, not silently defaulted by JSON
	// decoding. Null is never a substitute for captured persisted state.
	fields, err := requiredFields(b, "kind", "version", "list", "items", "stateFingerprint")
	if err != nil {
		return sp.ListDetail{}, err
	}
	if _, err := requiredFields(fields["list"], "id", "name", "createdAt", "updatedAt", "withdrawalGeneration"); err != nil {
		return sp.ListDetail{}, err
	}
	var items []json.RawMessage
	if err := json.Unmarshal(fields["items"], &items); err != nil {
		return sp.ListDetail{}, errors.New("invalid private snapshot members")
	}
	for _, i := range items {
		if _, err := requiredFields(i, "id", "purchaseId", "cardName", "certNumber", "grader", "grade", "addedAt", "packedAt", "version", "acknowledgedPriceCents", "acknowledgedStatus", "lastCommand"); err != nil {
			return sp.ListDetail{}, err
		}
	}
	return p.detail(), nil
}
func requiredFields(b []byte, keys ...string) (map[string]json.RawMessage, error) {
	var fields map[string]json.RawMessage
	if err := json.Unmarshal(b, &fields); err != nil {
		return nil, errors.New("private snapshot state must be an object")
	}
	for _, key := range keys {
		v, ok := fields[key]
		if !ok || bytes.Equal(bytes.TrimSpace(v), []byte("null")) {
			return nil, errors.New("private snapshot must include every persisted state field")
		}
	}
	return fields, nil
}
