package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	sp "github.com/guarzo/slabledger/internal/domain/showprep"
	"github.com/guarzo/slabledger/internal/testutil/mocks"
)

const listID = "33333333-3333-4333-8333-333333333333"
const itemID = "44444444-4444-4444-8444-444444444444"
const purchaseID = "11111111-1111-4111-8111-111111111111"
const commandID = "55555555-5555-4555-8555-555555555555"

func fixture() sp.ListDetail {
	d := sp.ListDetail{List: sp.List{ID: listID, Name: "Fixture show", CreatedAt: "2026-09-01T00:00:00Z", UpdatedAt: "2026-09-02T00:00:00Z"}, Items: []sp.Item{{ID: itemID, PurchaseID: purchaseID, CardName: "Card", CertNumber: "12345678", Grader: "PSA", Grade: 10, AddedAt: "2026-09-02T00:00:00Z", Version: 1}}}
	d.StateFingerprint = sp.ListStateFingerprint(d.List, d.Items)
	return d
}
func snapshotFile(t *testing.T, d sp.ListDetail) string {
	t.Helper()
	b, err := json.Marshal(d)
	if err != nil {
		t.Fatal(err)
	}
	return rawSnapshotFile(t, string(b))
}
func rawSnapshotFile(t *testing.T, b string) string {
	t.Helper()
	p := filepath.Join(t.TempDir(), "snapshot.json")
	if err := os.WriteFile(p, []byte(b), 0600); err != nil {
		t.Fatal(err)
	}
	return p
}
func readyPurchase() sp.Purchase {
	return sp.Purchase{ID: purchaseID, Known: true, Exists: true, CampaignExists: true, Phase: "active", Received: true, CertNumber: "12345678", Grader: "PSA", Grade: 10}
}
func readStore(d sp.ListDetail) *mocks.ShowPrepStoreMock {
	return &mocks.ShowPrepStoreMock{GetListFn: func(context.Context, string) (sp.List, error) { return d.List, nil }, GetItemsFn: func(context.Context, string) ([]sp.Item, error) { return d.Items, nil }, ReadPurchasesFn: func(context.Context, []string) (map[string]sp.Purchase, error) {
		return map[string]sp.Purchase{purchaseID: readyPurchase()}, nil
	}}
}

// Missing/invalid CLI input must fail before any DB connection, including secrets
// accidentally supplied as unsupported arguments.
func TestParse(t *testing.T) {
	for _, tc := range []struct {
		name      string
		args      []string
		ok, apply bool
	}{
		{"missing all", nil, false, false},
		{"missing operation", []string{"-snapshot", "/private/s.json", "-command-id", commandID}, false, false},
		{"missing snapshot", []string{"-operation", "confirm-existing", "-command-id", commandID}, false, false},
		{"missing command", []string{"-operation", "delete-empty", "-snapshot", "/private/s.json"}, false, false},
		{"bad operation", []string{"-operation", "bulk", "-snapshot", "/private/s.json", "-command-id", commandID}, false, false},
		{"bad uuid", []string{"-operation", "confirm-existing", "-snapshot", "/private/s.json", "-command-id", "not-a-uuid"}, false, false},
		{"noncanonical uuid", []string{"-operation", "confirm-existing", "-snapshot", "/private/s.json", "-command-id", "55555555555545558555555555555555"}, false, false},
		{"default dry run", []string{"-operation", "confirm-existing", "-snapshot", "/private/s.json", "-command-id", commandID}, true, false},
		{"explicit apply", []string{"-operation", "delete-empty", "-snapshot", "/private/s.json", "-command-id", commandID, "-apply"}, true, true},
		{"positional", []string{"-operation", "confirm-existing", "-snapshot", "/private/s.json", "-command-id", commandID, "secret"}, false, false},
		{"secret flag", []string{"-database-url", "postgres://secret"}, false, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			o, err := parse(tc.args)
			if (err == nil) != tc.ok {
				t.Fatalf("err=%v", err)
			}
			if tc.ok && o.apply != tc.apply {
				t.Fatalf("apply=%v", o.apply)
			}
			if err != nil && strings.Contains(err.Error(), "secret") {
				t.Fatal("secret leaked")
			}
		})
	}
}

func TestSnapshotValidation(t *testing.T) {
	for _, tc := range []struct {
		name      string
		change    func(*sp.ListDetail)
		operation string
		ok        bool
	}{
		{"valid", nil, "confirm-existing", true},
		{"altered name", func(d *sp.ListDetail) { d.List.Name = "Altered" }, "confirm-existing", false},
		{"altered cohort", func(d *sp.ListDetail) { d.Items[0].ID = commandID }, "confirm-existing", false},
		{"bad member id", func(d *sp.ListDetail) {
			d.Items[0].ID = "bad"
			d.StateFingerprint = sp.ListStateFingerprint(d.List, d.Items)
		}, "confirm-existing", false},
		{"duplicate member", func(d *sp.ListDetail) {
			d.Items = append(d.Items, d.Items[0])
			d.StateFingerprint = sp.ListStateFingerprint(d.List, d.Items)
		}, "confirm-existing", false},
		{"duplicate purchase", func(d *sp.ListDetail) {
			i := d.Items[0]
			i.ID = commandID
			d.Items = append(d.Items, i)
			d.StateFingerprint = sp.ListStateFingerprint(d.List, d.Items)
		}, "confirm-existing", false},
		{"null cohort", func(d *sp.ListDetail) { d.Items = nil; d.StateFingerprint = sp.ListStateFingerprint(d.List, d.Items) }, "confirm-existing", false},
		{"invalid generation", func(d *sp.ListDetail) {
			d.List.WithdrawalGeneration = -1
			d.StateFingerprint = sp.ListStateFingerprint(d.List, d.Items)
		}, "confirm-existing", false},
		{"delete nonempty", nil, "delete-empty", false},
		{"delete empty", func(d *sp.ListDetail) {
			d.Items = []sp.Item{}
			d.StateFingerprint = sp.ListStateFingerprint(d.List, d.Items)
		}, "delete-empty", true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			d := fixture()
			if tc.change != nil {
				tc.change(&d)
			}
			_, err := loadSnapshot(snapshotFile(t, d), tc.operation)
			if (err == nil) != tc.ok {
				t.Fatalf("err=%v", err)
			}
		})
	}
	valid, err := json.Marshal(fixture())
	if err != nil {
		t.Fatal(err)
	}
	body := string(valid)
	for _, tc := range []struct{ name, body string }{
		{"unknown", strings.TrimSuffix(body, "}") + `,"unknown":true}`},
		{"trailing", body + " {}"},
		{"duplicate", strings.Replace(body, `"name":"Fixture show"`, `"name":"Fixture show","name":"Fixture show"`, 1)},
		{"truncated", "{"}, {"null", "null"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if _, err := loadSnapshot(rawSnapshotFile(t, tc.body), "confirm-existing"); err == nil {
				t.Fatal("accepted malformed snapshot")
			}
		})
	}
}

// A dry run must not enter a session, observe associations, or read evidence.
func TestDryRun(t *testing.T) {
	for _, tc := range []struct {
		name      string
		change    func(*mocks.ShowPrepStoreMock)
		ok        bool
		wantError string
	}{
		{"ready without price or evidence", nil, true, ""},
		{"changed membership", func(s *mocks.ShowPrepStoreMock) {
			s.GetItemsFn = func(context.Context, string) ([]sp.Item, error) { return []sp.Item{}, nil }
		}, false, "snapshot/cohort changed"},
		{"changed list", func(s *mocks.ShowPrepStoreMock) {
			s.GetListFn = func(context.Context, string) (sp.List, error) { l := fixture().List; l.Name = "Changed"; return l, nil }
		}, false, "snapshot/cohort changed"},
		{"invalid member", func(s *mocks.ShowPrepStoreMock) {
			s.ReadPurchasesFn = func(context.Context, []string) (map[string]sp.Purchase, error) {
				p := readyPurchase()
				p.Received = false
				return map[string]sp.Purchase{purchaseID: p}, nil
			}
		}, false, "unavailable or identity-changed"},
		{"identity mismatch", func(s *mocks.ShowPrepStoreMock) {
			s.ReadPurchasesFn = func(context.Context, []string) (map[string]sp.Purchase, error) {
				p := readyPurchase()
				p.CertNumber = "87654321"
				return map[string]sp.Purchase{purchaseID: p}, nil
			}
		}, false, "unavailable or identity-changed"},
		{"read error redacted", func(s *mocks.ShowPrepStoreMock) {
			s.GetListFn = func(context.Context, string) (sp.List, error) {
				return sp.List{}, errors.New("postgres://secret-token")
			}
		}, false, "verify schema/connectivity"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			d := fixture()
			s := readStore(d)
			if tc.change != nil {
				tc.change(s)
			}
			forbidden := func() { t.Fatal("dry-run invoked a side-effect/evidence method") }
			s.WithinFn = func(context.Context, func(sp.Session) error) error { forbidden(); return nil }
			s.ObservePriceAssociationsFn = func(context.Context, []string) (map[string]bool, error) { forbidden(); return nil, nil }
			s.ReadSnapshotsFn = func(context.Context, []sp.Identity) (map[sp.Identity]*sp.Snapshot, error) {
				forbidden()
				return nil, nil
			}
			var out bytes.Buffer
			err := execute(context.Background(), options{operation: "confirm-existing", commandID: commandID}, d, s, nil, &out)
			if (err == nil) != tc.ok {
				t.Fatalf("err=%v", err)
			}
			if err != nil && (!strings.Contains(err.Error(), tc.wantError) || strings.Contains(err.Error(), "secret")) {
				t.Fatalf("wrong or unsafe error: %v", err)
			}
			if tc.ok && !strings.Contains(out.String(), "dry_run") {
				t.Fatalf("output=%s", out.String())
			}
		})
	}
}

func TestApplyAndReplay(t *testing.T) {
	for _, tc := range []struct {
		name, operation string
		replay, fail    bool
	}{
		{"confirm", "confirm-existing", false, false}, {"confirm replay", "confirm-existing", true, false}, {"delete", "delete-empty", false, false}, {"delete replay", "delete-empty", true, false}, {"failure", "confirm-existing", false, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			d := fixture()
			if tc.operation == "delete-empty" {
				d.Items = []sp.Item{}
				d.StateFingerprint = sp.ListStateFingerprint(d.List, d.Items)
			}
			svc := &mocks.ShowPrepTransitionServiceMock{
				ConfirmExistingFn: func(_ context.Context, id string, c sp.ConfirmExistingCommand) (sp.ConfirmExistingResult, error) {
					if id != listID || c.CommandID != commandID || c.ExpectedState != d.StateFingerprint || len(c.ItemIDs) != 1 || c.ItemIDs[0] != itemID {
						t.Fatal("wrong exact consent")
					}
					if tc.fail {
						return sp.ConfirmExistingResult{}, errors.New("secret DSN")
					}
					return sp.ConfirmExistingResult{ListID: id, CommandID: c.CommandID, ConfirmedItemIDs: c.ItemIDs, Replay: tc.replay}, nil
				},
				DeleteListFn: func(_ context.Context, id string, c sp.DeleteListCommand) (sp.DeleteListResult, error) {
					if id != listID || c.CommandID != commandID || c.ExpectedState != d.StateFingerprint {
						t.Fatal("wrong delete consent")
					}
					return sp.DeleteListResult{ListID: id, Deleted: true, Replay: tc.replay}, nil
				},
			}
			// No reads allowed before replay; a deleted/changed list must still retry.
			var out bytes.Buffer
			err := execute(context.Background(), options{operation: tc.operation, commandID: commandID, apply: true}, d, nil, svc, &out)
			if (err != nil) != tc.fail {
				t.Fatalf("err=%v", err)
			}
			if err != nil && strings.Contains(err.Error(), "secret") {
				t.Fatal("leaked secret")
			}
			if !tc.fail && !strings.Contains(out.String(), "\"replay\":"+map[bool]string{true: "true", false: "false"}[tc.replay]) {
				t.Fatalf("output=%s", out.String())
			}
		})
	}
}
