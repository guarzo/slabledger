package main

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"io"
	"slices"
	"strings"

	"github.com/google/uuid"

	sp "github.com/guarzo/slabledger/internal/domain/showprep"
)

type options struct {
	operation, snapshot, commandID string
	apply                          bool
}
type readStorePort interface {
	GetList(context.Context, string) (sp.List, error)
	GetItems(context.Context, string) ([]sp.Item, error)
	ReadPurchases(context.Context, []string) (map[string]sp.Purchase, error)
}
type transitionService interface {
	ConfirmExisting(context.Context, string, sp.ConfirmExistingCommand) (sp.ConfirmExistingResult, error)
	DeleteList(context.Context, string, sp.DeleteListCommand) (sp.DeleteListResult, error)
}

func parse(args []string) (options, error) {
	var o options
	fs := flag.NewFlagSet("showprep-transition", flag.ContinueOnError)
	fs.SetOutput(io.Discard) // The flag package includes supplied values in errors.
	fs.StringVar(&o.operation, "operation", "", "confirm-existing or delete-empty")
	fs.StringVar(&o.snapshot, "snapshot", "", "private full ListDetail JSON path")
	fs.StringVar(&o.commandID, "command-id", "", "canonical command UUID")
	fs.BoolVar(&o.apply, "apply", false, "apply atomic command (default dry-run)")
	if err := fs.Parse(args); err != nil || fs.NArg() != 0 {
		return options{}, errors.New("invalid flags; require -operation, -snapshot, -command-id; optional -apply")
	}
	if (o.operation != "confirm-existing" && o.operation != "delete-empty") || strings.TrimSpace(o.snapshot) == "" || !canonicalID(o.commandID) {
		return options{}, errors.New("require operation confirm-existing|delete-empty, snapshot path, and canonical command UUID")
	}
	return o, nil
}
func canonicalID(id string) bool { u, err := uuid.Parse(id); return err == nil && u.String() == id }

// Apply intentionally bypasses advisory reads. The domain checks receipt/tombstone
// before live state, preserving exact same-ID recovery after a lost response.
func execute(ctx context.Context, o options, d sp.ListDetail, store readStorePort, svc transitionService, out io.Writer) error {
	if err := validateSnapshot(d, o.operation); err != nil {
		return err
	}
	if !canonicalID(o.commandID) {
		return errors.New("invalid command UUID")
	}
	ids := make([]string, 0, len(d.Items))
	for _, i := range d.Items {
		ids = append(ids, i.ID)
	}
	slices.Sort(ids)
	if o.apply {
		var result any
		var err error
		if o.operation == "confirm-existing" {
			result, err = svc.ConfirmExisting(ctx, d.List.ID, sp.ConfirmExistingCommand{CommandID: o.commandID, ExpectedState: d.StateFingerprint, ItemIDs: ids})
		} else {
			result, err = svc.DeleteList(ctx, d.List.ID, sp.DeleteListCommand{CommandID: o.commandID, ExpectedState: d.StateFingerprint})
		}
		if err != nil {
			return commandError(err)
		}
		if err := json.NewEncoder(out).Encode(result); err != nil {
			return errors.New("command may have committed; output failed; retry unchanged snapshot with same command UUID")
		}
		return nil
	}
	list, err := store.GetList(ctx, d.List.ID)
	if err != nil {
		return commandError(err)
	}
	items, err := store.GetItems(ctx, d.List.ID)
	if err != nil {
		return commandError(err)
	}
	if sp.ListStateFingerprint(list, items) != d.StateFingerprint {
		return commandError(sp.ErrConflict)
	}
	if o.operation == "delete-empty" && len(items) != 0 {
		return errors.New("delete-empty requires empty current membership")
	}
	purchases := make([]string, 0, len(items))
	for _, i := range items {
		purchases = append(purchases, i.PurchaseID)
	}
	ps, err := store.ReadPurchases(ctx, purchases)
	if err != nil {
		return commandError(err)
	}
	for _, i := range items {
		p := ps[i.PurchaseID]
		if sp.AvailabilityOf(p) != sp.Ready || i.CertNumber == "" || p.CertNumber != i.CertNumber || !strings.EqualFold(strings.TrimSpace(i.Grader), p.Grader) || p.Grade != i.Grade {
			return errors.New("cohort contains unavailable or identity-changed member; no writes performed")
		}
	}
	if err := json.NewEncoder(out).Encode(struct {
		Mode      string   `json:"mode"`
		Operation string   `json:"operation"`
		ListID    string   `json:"listId"`
		CommandID string   `json:"commandId"`
		ItemIDs   []string `json:"itemIds"`
	}{"dry_run", o.operation, d.List.ID, o.commandID, ids}); err != nil {
		return errors.New("dry-run output failed; no writes performed")
	}
	return nil
}

// Never propagate arbitrary storage errors: PostgreSQL errors can contain DSNs,
// credential material, or private SQL values. Known domain errors remain useful.
func commandError(err error) error {
	switch {
	case errors.Is(err, sp.ErrContended):
		return errors.New("state contended; retry unchanged command UUID")
	case errors.Is(err, sp.ErrRetired):
		return errors.New("show retired; command did not match tombstone")
	case errors.Is(err, sp.ErrConflict):
		return errors.New("snapshot/cohort changed or command UUID payload mismatch")
	case errors.Is(err, sp.ErrNotFound):
		return errors.New("show not found")
	case errors.Is(err, sp.ErrInvalid):
		return errors.New("invalid transition command")
	default:
		return errors.New("database operation failed; verify schema/connectivity; if applying, retry unchanged snapshot with same command UUID")
	}
}
