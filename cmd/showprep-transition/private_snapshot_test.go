package main

import (
	"bytes"
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	sp "github.com/guarzo/slabledger/internal/domain/showprep"
	"github.com/stretchr/testify/require"
)

func privateBody(t *testing.T, d sp.ListDetail) string {
	t.Helper()
	items := make([]map[string]any, 0, len(d.Items))
	for _, i := range d.Items {
		items = append(items, map[string]any{"id": i.ID, "purchaseId": i.PurchaseID, "cardName": i.CardName, "certNumber": i.CertNumber, "grader": i.Grader, "grade": i.Grade, "addedAt": i.AddedAt, "packedAt": i.PackedAt, "version": i.Version, "acknowledgedPriceCents": i.AcknowledgedPriceCents, "acknowledgedStatus": i.AcknowledgedStatus, "lastCommand": i.LastCommand})
	}
	b, err := json.Marshal(map[string]any{"kind": "showprep-transition-snapshot", "version": 1, "list": d.List, "items": items, "stateFingerprint": d.StateFingerprint})
	require.NoError(t, err)
	return string(b)
}

func TestPrivateSnapshotLegacyRoundTrip(t *testing.T) {
	for _, last := range []string{"", sp.Fingerprint("legacy command")} {
		t.Run(last, func(t *testing.T) {
			d := fixture()
			d.Items[0].LastCommand = last
			d.StateFingerprint = sp.ListStateFingerprint(d.List, d.Items)
			loaded, err := loadSnapshot(rawSnapshotFile(t, privateBody(t, d)), "confirm-existing")
			require.NoError(t, err)
			require.Equal(t, d, loaded)
			var out bytes.Buffer
			require.NoError(t, execute(context.Background(), options{operation: "confirm-existing", commandID: commandID}, loaded, readStore(d), nil, &out))
			require.Contains(t, out.String(), `"mode":"dry_run"`)
		})
	}
}

func TestExportParser(t *testing.T) {
	for _, tc := range []struct {
		name string
		args []string
		ok   bool
	}{
		{"valid", []string{"-operation", "export", "-list-id", listID, "-snapshot", "/private/s.json"}, true},
		{"missing list", []string{"-operation", "export", "-snapshot", "/private/s.json"}, false},
		{"bad list", []string{"-operation", "export", "-list-id", "bad", "-snapshot", "/private/s.json"}, false},
		{"apply forbidden", []string{"-operation", "export", "-list-id", listID, "-snapshot", "/private/s.json", "-apply"}, false},
		{"command forbidden", []string{"-operation", "export", "-list-id", listID, "-snapshot", "/private/s.json", "-command-id", commandID}, false},
		{"transition list flag forbidden", []string{"-operation", "confirm-existing", "-list-id", listID, "-snapshot", "/private/s.json", "-command-id", commandID}, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			_, err := parse(tc.args)
			if tc.ok {
				require.NoError(t, err)
			} else {
				require.Error(t, err)
			}
		})
	}
}

func TestPrivateSnapshotStrict(t *testing.T) {
	d := fixture()
	d.Items[0].LastCommand = sp.Fingerprint("legacy")
	d.StateFingerprint = sp.ListStateFingerprint(d.List, d.Items)
	for _, tc := range []struct {
		name   string
		change func(map[string]any)
	}{
		{"wrong kind", func(m map[string]any) { m["kind"] = "other" }},
		{"wrong version", func(m map[string]any) { m["version"] = 2 }},
		{"missing last command", func(m map[string]any) { delete(m["items"].([]any)[0].(map[string]any), "lastCommand") }},
		{"altered last command", func(m map[string]any) { m["items"].([]any)[0].(map[string]any)["lastCommand"] = "altered" }},
		{"missing zero field", func(m map[string]any) { delete(m["list"].(map[string]any), "withdrawalGeneration") }},
		{"unknown field", func(m map[string]any) { m["items"].([]any)[0].(map[string]any)["unknown"] = true }},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var m map[string]any
			require.NoError(t, json.Unmarshal([]byte(privateBody(t, d)), &m))
			tc.change(m)
			b, err := json.Marshal(m)
			require.NoError(t, err)
			_, err = loadSnapshot(rawSnapshotFile(t, string(b)), "confirm-existing")
			require.Error(t, err)
		})
	}
}

func TestExportReadOnlyAndPrivateFile(t *testing.T) {
	for _, last := range []string{"", sp.Fingerprint("legacy")} {
		t.Run(last, func(t *testing.T) {
			d := fixture()
			d.Items[0].LastCommand = last
			d.StateFingerprint = sp.ListStateFingerprint(d.List, d.Items)
			s := readStore(d)
			s.ReadPurchasesFn = func(context.Context, []string) (map[string]sp.Purchase, error) {
				t.Fatal("export read purchases")
				return nil, nil
			}
			s.WithinFn = func(context.Context, func(sp.Session) error) error {
				t.Fatal("export opened mutation session")
				return nil
			}
			s.ReadSnapshotsFn = func(context.Context, []sp.Identity) (map[sp.Identity]*sp.Snapshot, error) {
				t.Fatal("export read evidence")
				return nil, nil
			}
			s.ObservePriceAssociationsFn = func(context.Context, []string) (map[string]bool, error) {
				t.Fatal("export observed associations")
				return nil, nil
			}
			path := filepath.Join(t.TempDir(), "private.json")
			require.NoError(t, exportSnapshot(context.Background(), listID, path, s))
			info, err := os.Stat(path)
			require.NoError(t, err)
			require.Equal(t, os.FileMode(0600), info.Mode().Perm())
			loaded, err := loadSnapshot(path, "confirm-existing")
			require.NoError(t, err)
			require.Equal(t, d, loaded)
			before, err := os.ReadFile(path)
			require.NoError(t, err)
			require.Error(t, exportSnapshot(context.Background(), listID, path, s), "must not overwrite captured consent")
			after, err := os.ReadFile(path)
			require.NoError(t, err)
			require.Equal(t, before, after)
			require.Error(t, exportSnapshot(context.Background(), "bad", filepath.Join(t.TempDir(), "invalid.json"), s))
		})
	}
}
