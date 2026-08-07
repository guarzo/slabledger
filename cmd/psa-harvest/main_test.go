package main

import (
	"errors"
	"strings"
	"testing"

	"github.com/guarzo/slabledger/internal/adapters/clients/psaportal"
)

func TestDecideReconcileGate(t *testing.T) {
	tests := []struct {
		name           string
		campaignsFresh bool
		rowsErr        error
		wantProceed    bool
		wantErrIs      error // if set, gate.skipErr must satisfy errors.Is against this
		wantMsgHasWord string
	}{
		{
			name:           "campaign snapshot not written this run skips, names the campaign snapshot",
			campaignsFresh: false,
			rowsErr:        nil,
			wantProceed:    false,
			wantMsgHasWord: "campaign snapshot",
		},
		{
			name:           "itemized rows stale skips, names the itemized snapshot via the sentinel error",
			campaignsFresh: true,
			rowsErr:        psaportal.ErrStaleSnapshot,
			wantProceed:    false,
			wantErrIs:      psaportal.ErrStaleSnapshot,
		},
		{
			name:           "both fresh proceeds",
			campaignsFresh: true,
			rowsErr:        nil,
			wantProceed:    true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := decideReconcileGate(tt.campaignsFresh, tt.rowsErr)

			if got.proceed != tt.wantProceed {
				t.Fatalf("proceed = %v, want %v", got.proceed, tt.wantProceed)
			}
			if tt.wantProceed {
				if got.skipMsg != "" || got.skipErr != nil {
					t.Fatalf("expected no skip reason when proceeding, got skipMsg=%q skipErr=%v", got.skipMsg, got.skipErr)
				}
				return
			}
			if got.skipMsg == "" {
				t.Fatal("expected a non-empty skipMsg when skipping")
			}
			if tt.wantMsgHasWord != "" && !strings.Contains(got.skipMsg, tt.wantMsgHasWord) {
				t.Fatalf("skipMsg %q does not mention %q", got.skipMsg, tt.wantMsgHasWord)
			}
			if tt.wantErrIs != nil && !errors.Is(got.skipErr, tt.wantErrIs) {
				t.Fatalf("skipErr = %v, want errors.Is match for %v", got.skipErr, tt.wantErrIs)
			}
		})
	}
}
