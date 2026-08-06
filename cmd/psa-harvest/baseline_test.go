package main

import (
	"context"
	"errors"
	"reflect"
	"testing"

	"github.com/guarzo/slabledger/internal/domain/inventory"
	"github.com/guarzo/slabledger/internal/domain/observability"
	"github.com/guarzo/slabledger/internal/domain/psacampaign"
	"github.com/guarzo/slabledger/internal/testutil/mocks"
)

func TestParseBaselineFlag(t *testing.T) {
	tests := []struct {
		name         string
		args         []string
		wantBaseline bool
		wantRest     []string
		wantErr      bool
	}{
		{
			name:         "no flags",
			args:         []string{},
			wantBaseline: false,
			wantRest:     []string{},
		},
		{
			name:         "baseline flag alone",
			args:         []string{"-baseline-pull"},
			wantBaseline: true,
			wantRest:     []string{},
		},
		{
			name:         "double-dash form",
			args:         []string{"--baseline-pull"},
			wantBaseline: true,
			wantRest:     []string{},
		},
		{
			name:         "explicit false is filtered but not set",
			args:         []string{"-baseline-pull=false", "-log-level", "debug"},
			wantBaseline: false,
			wantRest:     []string{"-log-level", "debug"},
		},
		{
			name:         "baseline flag mixed with unrelated flags config.Load must still see",
			args:         []string{"-log-level", "debug", "-baseline-pull", "-cache", "/tmp/cache.json"},
			wantBaseline: true,
			wantRest:     []string{"-log-level", "debug", "-cache", "/tmp/cache.json"},
		},
		{
			// The safety case: a typo must abort the run, never silently
			// degrade to the write-enabled mode that drains the push queue.
			name:    "malformed value is an error, not a fallback to write mode",
			args:    []string{"-baseline-pull=ture"},
			wantErr: true,
		},
		{
			name:    "malformed value on the double-dash form is also an error",
			args:    []string{"--baseline-pull=maybe", "-log-level", "debug"},
			wantErr: true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			baseline, rest, err := parseBaselineFlag(tt.args)
			if tt.wantErr {
				if err == nil {
					t.Fatalf("parseBaselineFlag(%v): got nil error, want error", tt.args)
				}
				if baseline {
					t.Errorf("baseline = true on error, want false — a failed parse must never enable a mode")
				}
				return
			}
			if err != nil {
				t.Fatalf("parseBaselineFlag(%v): unexpected error: %v", tt.args, err)
			}
			if baseline != tt.wantBaseline {
				t.Errorf("baseline = %v, want %v", baseline, tt.wantBaseline)
			}
			if !reflect.DeepEqual(rest, tt.wantRest) {
				t.Errorf("rest = %v, want %v", rest, tt.wantRest)
			}
		})
	}
}

func TestBaselineLanguage(t *testing.T) {
	tests := []struct {
		name          string
		specListNames []string
		want          string
		wantErr       bool
	}{
		{name: "japanese", specListNames: []string{"Japanese Pokemon"}, want: "japanese"},
		{name: "english", specListNames: []string{"English Pokemon"}, want: "english"},
		{name: "no recognized name", specListNames: []string{}, wantErr: true},
		{name: "unrecognized name only", specListNames: []string{"Something Else"}, wantErr: true},
		{
			name:          "both recognized names is ambiguous",
			specListNames: []string{"Japanese Pokemon", "English Pokemon"},
			wantErr:       true,
		},
		{
			name:          "duplicate of the same name is not ambiguous",
			specListNames: []string{"Japanese Pokemon", "Japanese Pokemon"},
			want:          "japanese",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := baselineLanguage(tt.specListNames)
			if tt.wantErr {
				if err == nil {
					t.Fatalf("baselineLanguage(%v) = %q, nil; want error", tt.specListNames, got)
				}
				return
			}
			if err != nil {
				t.Fatalf("baselineLanguage(%v): unexpected error: %v", tt.specListNames, err)
			}
			if got != tt.want {
				t.Errorf("baselineLanguage(%v) = %q, want %q", tt.specListNames, got, tt.want)
			}
		})
	}
}

func TestBuildBaselineCampaign(t *testing.T) {
	existing := inventory.Campaign{ID: "camp-1", Name: "Vintage Core", PSACampaignRequestID: "req-1"}

	tests := []struct {
		name    string
		pc      psacampaign.PortalCampaign
		want    inventory.Campaign
		wantErr bool
	}{
		{
			name: "japanese target campaign copies subjects and denied specs verbatim",
			pc: psacampaign.PortalCampaign{
				CampaignRequestID: "req-1",
				SpecListNames:     []string{"Japanese Pokemon"},
				SubjectFilter: psacampaign.CampaignFilter{
					Type:     "Target",
					Subjects: []psacampaign.SubjectRef{{ID: 22210, Name: "Machamp"}, {ID: 8105, Name: "Crystal Golem"}},
				},
				DeniedSpecs: []psacampaign.SubjectRef{{ID: 4807, Name: "Gold Star Charizard"}},
			},
			want: inventory.Campaign{
				ID: "camp-1", Name: "Vintage Core", PSACampaignRequestID: "req-1",
				TargetLanguage:    "japanese",
				SubjectFilterMode: "Target",
				Subjects: []inventory.TargetSubject{
					{ID: 22210, Name: "Machamp"},
					{ID: 8105, Name: "Crystal Golem"},
				},
				DeniedSpecs: []inventory.TargetSubject{{ID: 4807, Name: "Gold Star Charizard"}},
			},
		},
		{
			name: "exclude with zero subjects is an open net, not an error",
			pc: psacampaign.PortalCampaign{
				CampaignRequestID: "req-1",
				SpecListNames:     []string{"English Pokemon"},
				SubjectFilter:     psacampaign.CampaignFilter{Type: "Exclude"},
			},
			want: inventory.Campaign{
				ID: "camp-1", Name: "Vintage Core", PSACampaignRequestID: "req-1",
				TargetLanguage:    "english",
				SubjectFilterMode: "Exclude",
				Subjects:          []inventory.TargetSubject{},
				DeniedSpecs:       []inventory.TargetSubject{},
			},
		},
		{
			name: "unmappable language is an error, existing campaign untouched",
			pc: psacampaign.PortalCampaign{
				CampaignRequestID: "req-1",
				SpecListNames:     []string{},
			},
			wantErr: true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := buildBaselineCampaign(existing, tt.pc)
			if tt.wantErr {
				if err == nil {
					t.Fatalf("buildBaselineCampaign(): got nil error, want error")
				}
				return
			}
			if err != nil {
				t.Fatalf("buildBaselineCampaign(): unexpected error: %v", err)
			}
			if got.TargetLanguage != tt.want.TargetLanguage ||
				got.SubjectFilterMode != tt.want.SubjectFilterMode ||
				len(got.Subjects) != len(tt.want.Subjects) ||
				len(got.DeniedSpecs) != len(tt.want.DeniedSpecs) {
				t.Fatalf("buildBaselineCampaign() = %+v, want %+v", got, tt.want)
			}
			for i := range tt.want.Subjects {
				if got.Subjects[i] != tt.want.Subjects[i] {
					t.Errorf("Subjects[%d] = %+v, want %+v", i, got.Subjects[i], tt.want.Subjects[i])
				}
			}
			for i := range tt.want.DeniedSpecs {
				if got.DeniedSpecs[i] != tt.want.DeniedSpecs[i] {
					t.Errorf("DeniedSpecs[%d] = %+v, want %+v", i, got.DeniedSpecs[i], tt.want.DeniedSpecs[i])
				}
			}
			if got.ID != tt.want.ID || got.Name != tt.want.Name || got.PSACampaignRequestID != tt.want.PSACampaignRequestID {
				t.Errorf("existing campaign fields altered: got %+v", got)
			}
		})
	}
}

func TestRunBaselinePull(t *testing.T) {
	linkedComplete := psacampaign.PortalCampaign{
		CampaignRequestID: "req-1", TargetingComplete: true,
		SpecListNames: []string{"Japanese Pokemon"},
		SubjectFilter: psacampaign.CampaignFilter{Type: "Target"},
	}
	linkedIncomplete := psacampaign.PortalCampaign{CampaignRequestID: "req-2", TargetingComplete: false}
	// Same shape as linkedComplete (a cleanly resolvable language and subject
	// filter) except for TargetingComplete, so a case built from it isolates
	// the TargetingComplete guard: if that guard were ever bypassed, this
	// fixture would resolve and write instead of skip, unlike linkedIncomplete
	// (which also fails on empty SpecListNames and would mask the bypass).
	linkedIncompleteOtherwiseValid := psacampaign.PortalCampaign{
		CampaignRequestID: "req-1", TargetingComplete: false,
		SpecListNames: []string{"Japanese Pokemon"},
		SubjectFilter: psacampaign.CampaignFilter{Type: "Target"},
	}
	linkedAmbiguousLanguage := psacampaign.PortalCampaign{
		CampaignRequestID: "req-3", TargetingComplete: true,
		SpecListNames: []string{}, // no recognized name -> unconverted CATEGORY campaign, §8
	}
	notLinked := psacampaign.PortalCampaign{CampaignRequestID: "req-unlinked", TargetingComplete: true}

	// The internal fleet is per-case, not a shared fixture: the unobserved-links
	// check makes the result depend on which internal campaigns exist, so a case
	// that passes only one portal campaign must also narrow the fleet or it will
	// (correctly) report the other two as missing from the portal.
	allThree := []inventory.Campaign{
		{ID: "camp-1", PSACampaignRequestID: "req-1"},
		{ID: "camp-2", PSACampaignRequestID: "req-2"},
		{ID: "camp-3", PSACampaignRequestID: "req-3"},
	}
	onlyOne := []inventory.Campaign{{ID: "camp-1", PSACampaignRequestID: "req-1"}}

	tests := []struct {
		name       string
		internal   []inventory.Campaign
		portal     []psacampaign.PortalCampaign
		updateErr  error
		wantErr    bool
		wantWrites int
	}{
		{
			name:       "writes the linked complete campaign, skips the rest, exits non-zero",
			internal:   allThree,
			portal:     []psacampaign.PortalCampaign{linkedComplete, linkedIncomplete, linkedAmbiguousLanguage, notLinked},
			wantErr:    true,
			wantWrites: 1,
		},
		{
			name:       "all campaigns clean is a nil error",
			internal:   onlyOne,
			portal:     []psacampaign.PortalCampaign{linkedComplete},
			wantErr:    false,
			wantWrites: 1,
		},
		{
			name:       "an update failure aborts immediately",
			internal:   onlyOne,
			portal:     []psacampaign.PortalCampaign{linkedComplete},
			updateErr:  errors.New("db down"),
			wantErr:    true,
			wantWrites: 1,
		},
		{
			// Isolates the TargetingComplete guard from every other skip
			// reason: SpecListNames/SubjectFilter here resolve cleanly, so the
			// only thing that can cause a skip (writes == 0) is the guard
			// itself. A bypassed guard would write and this case would fail.
			name:       "incomplete edit-form fetch skips even when the rest of the record is resolvable",
			internal:   onlyOne,
			portal:     []psacampaign.PortalCampaign{linkedIncompleteOtherwiseValid},
			wantErr:    true,
			wantWrites: 0,
		},
		{
			// The blind spot the loop cannot see: camp-2 and camp-3 are linked
			// but the portal never returned them, so they keep stale targeting.
			// Without the unobserved check this case returns nil and exits 0.
			name:       "linked campaigns absent from the portal fetch are an error",
			internal:   allThree,
			portal:     []psacampaign.PortalCampaign{linkedComplete},
			wantErr:    true,
			wantWrites: 1,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			writes := 0
			repo := &mocks.CampaignRepositoryMock{
				ListCampaignsFn: func(ctx context.Context, activeOnly bool) ([]inventory.Campaign, error) {
					return tt.internal, nil
				},
				UpdateCampaignFn: func(ctx context.Context, c *inventory.Campaign) error {
					writes++
					return tt.updateErr
				},
			}
			logger := observability.NewNoopLogger()
			err := runBaselinePull(context.Background(), tt.portal, repo, logger)
			if tt.wantErr && err == nil {
				t.Fatalf("runBaselinePull(): got nil error, want error")
			}
			if !tt.wantErr && err != nil {
				t.Fatalf("runBaselinePull(): unexpected error: %v", err)
			}
			if writes != tt.wantWrites {
				t.Errorf("UpdateCampaign called %d times, want %d", writes, tt.wantWrites)
			}
		})
	}
}
