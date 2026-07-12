package psaportal

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/guarzo/slabledger/internal/domain/observability"
	"github.com/guarzo/slabledger/internal/testutil/mocks"
)

func TestSnapshotRowProvider_FetchRows(t *testing.T) {
	goodRow := map[string]string{
		colCert:      "12345678",
		colTitle:     "Charizard Base Set",
		colGrade:     "9.0",
		colPricePaid: "$150.00",
		colDate:      "2026-07-10",
	}
	badGradeRow := map[string]string{colCert: "22222222", colGrade: "not-a-number"}
	emptyCertRow := map[string]string{colTitle: "no cert"}

	tests := []struct {
		name        string
		rows        []map[string]string
		fetchedAt   time.Time
		wantCount   int
		wantErrPart string
	}{
		{
			name:      "fresh snapshot maps rows",
			rows:      []map[string]string{goodRow},
			fetchedAt: time.Now().Add(-1 * time.Hour),
			wantCount: 1,
		},
		{
			name:      "malformed and empty-cert rows are skipped",
			rows:      []map[string]string{goodRow, badGradeRow, emptyCertRow},
			fetchedAt: time.Now().Add(-1 * time.Hour),
			wantCount: 1,
		},
		{
			name:        "all rows malformed is an error",
			rows:        []map[string]string{badGradeRow},
			fetchedAt:   time.Now().Add(-1 * time.Hour),
			wantErrPart: "failed to map",
		},
		{
			name:        "missing snapshot is an error",
			rows:        nil,
			fetchedAt:   time.Time{},
			wantErrPart: "no rows snapshot stored",
		},
		{
			name:        "stale snapshot is an error",
			rows:        []map[string]string{goodRow},
			fetchedAt:   time.Now().Add(-27 * time.Hour),
			wantErrPart: "stale",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			store := &mocks.PSASnapshotStoreMock{
				CurrentSnapshotFn: func(_ context.Context) ([]map[string]string, time.Time, error) {
					return tt.rows, tt.fetchedAt, nil
				},
			}
			p := NewSnapshotRowProvider(store, observability.NewNoopLogger())

			got, err := p.FetchRows(context.Background())
			if tt.wantErrPart != "" {
				if err == nil || !strings.Contains(err.Error(), tt.wantErrPart) {
					t.Fatalf("expected error containing %q, got %v", tt.wantErrPart, err)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if len(got) != tt.wantCount {
				t.Fatalf("expected %d rows, got %d", tt.wantCount, len(got))
			}
			if got[0].CertNumber != "12345678" {
				t.Errorf("expected cert 12345678, got %q", got[0].CertNumber)
			}
		})
	}
}
