package main

import (
	"context"
	"errors"
	"reflect"
	"strings"
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

func TestBaselineLanguages(t *testing.T) {
	tests := []struct {
		name          string
		specListNames []string
		want          []string
		wantErr       error
	}{
		{name: "japanese only", specListNames: []string{"Pokemon - Japanese Language Only"}, want: []string{"japanese"}},
		{name: "english only", specListNames: []string{"Pokemon - English Language Only"}, want: []string{"english"}},
		{
			// The live shape: all six portal campaigns carry both lists. The
			// old code rejected this as ambiguous, which is why the baseline
			// pull could not run at all.
			name:          "both lists is the live shape, not an error",
			specListNames: []string{"Pokemon - Japanese Language Only", "Pokemon - English Language Only"},
			want:          []string{"english", "japanese"},
		},
		{
			name:          "order is normalized, not preserved",
			specListNames: []string{"Pokemon - English Language Only", "Pokemon - Japanese Language Only"},
			want:          []string{"english", "japanese"},
		},
		{
			name:          "duplicates collapse to one token",
			specListNames: []string{"Pokemon - Japanese Language Only", "Pokemon - Japanese Language Only"},
			want:          []string{"japanese"},
		},
		{
			// CATEGORY-era campaign: names no curated list at all. Expected,
			// and distinct from an unmodelled list.
			name:          "no names at all is the CATEGORY-era case",
			specListNames: []string{},
			wantErr:       errNoSpecListName,
		},
		{
			name:          "nil names is the CATEGORY-era case",
			specListNames: nil,
			wantErr:       errNoSpecListName,
		},
		{
			// Locked decision 4: a curated list SlabLedger does not model is
			// refused, never silently dropped.
			name:          "unmodelled list alone is refused",
			specListNames: []string{"English Base Set"},
			wantErr:       errUnrecognizedSpecListName,
		},
		{
			// The dangerous case: baselining this campaign from the two names
			// we do understand would record a narrower buy scope than the
			// portal actually has.
			name:          "unmodelled list alongside modelled ones is still refused",
			specListNames: []string{"Pokemon - Japanese Language Only", "English Base Set", "Pokemon - English Language Only"},
			wantErr:       errUnrecognizedSpecListName,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := baselineLanguages(tt.specListNames)
			if tt.wantErr != nil {
				if !errors.Is(err, tt.wantErr) {
					t.Fatalf("baselineLanguages(%v) error = %v, want errors.Is(_, %v)", tt.specListNames, err, tt.wantErr)
				}
				if got != nil {
					t.Errorf("baselineLanguages(%v) = %v on error, want nil", tt.specListNames, got)
				}
				return
			}
			if err != nil {
				t.Fatalf("baselineLanguages(%v): unexpected error: %v", tt.specListNames, err)
			}
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("baselineLanguages(%v) = %v, want %v", tt.specListNames, got, tt.want)
			}
		})
	}
}

func TestBaselineLanguagesErrorNamesTheList(t *testing.T) {
	// The operator's only lead on a refused campaign is this message; it must
	// carry the list name, not just the fact that something was unmodelled.
	_, err := baselineLanguages([]string{"English Base Set", "Pokemon - Japanese Language Only"})
	if err == nil {
		t.Fatal("baselineLanguages(): got nil error, want error")
	}
	if !strings.Contains(err.Error(), "English Base Set") {
		t.Errorf("error %q does not name the unrecognized list", err)
	}
}

func TestBuildBaselineCampaign(t *testing.T) {
	// Name is required: buildBaselineCampaign now runs the result through
	// inventory.ValidateAndNormalizeCampaign, which rejects an empty Name.
	existing := inventory.Campaign{ID: "camp-1", Name: "Vintage Core", PSACampaignRequestID: "req-1"}

	tests := []struct {
		name string
		// existingSubjects overrides existing.Subjects for this case only; nil
		// means use the shared existing fixture's (empty) Subjects.
		existingSubjects []inventory.TargetSubject
		pc               psacampaign.PortalCampaign
		want             inventory.Campaign
		wantErr          error
	}{
		{
			name: "both curated lists, subjects and denied specs copied verbatim",
			pc: psacampaign.PortalCampaign{
				CampaignRequestID: "req-1",
				SpecListIDs:       []string{"uuid-jp", "uuid-en"},
				SpecListNames:     []string{"Pokemon - Japanese Language Only", "Pokemon - English Language Only"},
				SubjectFilter: psacampaign.CampaignFilter{
					Type:     "Target",
					Subjects: []psacampaign.SubjectRef{{ID: 22210, Name: "Machamp"}, {ID: 8105, Name: "Crystal Golem"}},
				},
				DeniedSpecs: []psacampaign.SubjectRef{{ID: 4807, Name: "Gold Star Charizard"}},
			},
			want: inventory.Campaign{
				ID: "camp-1", Name: "Vintage Core", PSACampaignRequestID: "req-1",
				TargetLanguages:   []string{"english", "japanese"},
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
				SpecListIDs:       []string{"uuid-en"},
				SpecListNames:     []string{"Pokemon - English Language Only"},
				SubjectFilter:     psacampaign.CampaignFilter{Type: "Exclude"},
			},
			want: inventory.Campaign{
				ID: "camp-1", Name: "Vintage Core", PSACampaignRequestID: "req-1",
				TargetLanguages:   []string{"english"},
				SubjectFilterMode: "Exclude",
				Subjects:          []inventory.TargetSubject{},
				DeniedSpecs:       []inventory.TargetSubject{},
			},
		},
		{
			name: "no curated list is the CATEGORY-era skip",
			pc: psacampaign.PortalCampaign{
				CampaignRequestID: "req-1",
				SpecListIDs:       []string{},
				SpecListNames:     []string{},
			},
			wantErr: errNoSpecListName,
		},
		{
			// The catalog explained neither id, so SpecListNames came back
			// empty and baselineLanguages would have reported the harmless
			// CATEGORY-era case for a campaign that in fact buys two lists.
			name: "spec-list ids the catalog could not explain are refused",
			pc: psacampaign.PortalCampaign{
				CampaignRequestID: "req-1",
				SpecListIDs:       []string{"uuid-unknown-a", "uuid-unknown-b"},
				SpecListNames:     []string{},
				SubjectFilter:     psacampaign.CampaignFilter{Type: "Target"},
			},
			wantErr: errUnexplainedSpecListID,
		},
		{
			name: "one unexplained id alongside two explained ones is refused",
			pc: psacampaign.PortalCampaign{
				CampaignRequestID: "req-1",
				SpecListIDs:       []string{"uuid-jp", "uuid-en", "uuid-unknown"},
				SpecListNames:     []string{"Pokemon - Japanese Language Only", "Pokemon - English Language Only"},
				SubjectFilter:     psacampaign.CampaignFilter{Type: "Target"},
			},
			wantErr: errUnexplainedSpecListID,
		},
		{
			name: "unmodelled curated list is refused",
			pc: psacampaign.PortalCampaign{
				CampaignRequestID: "req-1",
				SpecListIDs:       []string{"uuid-en-base"},
				SpecListNames:     []string{"English Base Set"},
				SubjectFilter:     psacampaign.CampaignFilter{Type: "Target"},
			},
			wantErr: errUnrecognizedSpecListName,
		},
		{
			// The unvalidated-remote-string hole: anything but "Exclude" was
			// read as Target semantics by SubjectAxisMatches, so this string
			// used to reach a live buy decision unchecked.
			name: "unknown subject filter type is rejected by validation, not silently treated as Target",
			pc: psacampaign.PortalCampaign{
				CampaignRequestID: "req-1",
				SpecListIDs:       []string{"uuid-en"},
				SpecListNames:     []string{"Pokemon - English Language Only"},
				SubjectFilter: psacampaign.CampaignFilter{
					Type:     "Include",
					Subjects: []psacampaign.SubjectRef{{ID: 22210, Name: "Machamp"}},
				},
			},
			wantErr: inventory.ErrInvalidSubjectFilterMode,
		},
		{
			// The remedy this task names: a campaign carrying migration
			// 000023's -1 placeholder (inventory.LegacyUnreconciledSubjectID)
			// must come out the other side with only portal-supplied ids.
			// buildBaselineCampaign replaces Subjects wholesale rather than
			// merging into the existing slice, so the -1 placeholder cannot
			// survive a successful baseline pull.
			name:             "legacy unreconciled subject placeholder is replaced, not merged",
			existingSubjects: []inventory.TargetSubject{{ID: inventory.LegacyUnreconciledSubjectID, Name: "Charizard"}},
			pc: psacampaign.PortalCampaign{
				CampaignRequestID: "req-1",
				SpecListIDs:       []string{"uuid-en"},
				SpecListNames:     []string{"Pokemon - English Language Only"},
				SubjectFilter: psacampaign.CampaignFilter{
					Type:     "Target",
					Subjects: []psacampaign.SubjectRef{{ID: 22210, Name: "Machamp"}},
				},
			},
			want: inventory.Campaign{
				ID: "camp-1", Name: "Vintage Core", PSACampaignRequestID: "req-1",
				TargetLanguages:   []string{"english"},
				SubjectFilterMode: "Target",
				Subjects:          []inventory.TargetSubject{{ID: 22210, Name: "Machamp"}},
				DeniedSpecs:       []inventory.TargetSubject{},
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			e := existing
			if tt.existingSubjects != nil {
				e.Subjects = tt.existingSubjects
			}
			got, err := buildBaselineCampaign(e, tt.pc)
			if tt.wantErr != nil {
				if !errors.Is(err, tt.wantErr) {
					t.Fatalf("buildBaselineCampaign() error = %v, want errors.Is(_, %v)", err, tt.wantErr)
				}
				if !reflect.DeepEqual(got, inventory.Campaign{}) {
					t.Errorf("buildBaselineCampaign() = %+v on error, want zero Campaign", got)
				}
				return
			}
			if err != nil {
				t.Fatalf("buildBaselineCampaign(): unexpected error: %v", err)
			}
			if !reflect.DeepEqual(got.TargetLanguages, tt.want.TargetLanguages) {
				t.Errorf("TargetLanguages = %v, want %v", got.TargetLanguages, tt.want.TargetLanguages)
			}
			if got.SubjectFilterMode != tt.want.SubjectFilterMode {
				t.Errorf("SubjectFilterMode = %q, want %q", got.SubjectFilterMode, tt.want.SubjectFilterMode)
			}
			if !reflect.DeepEqual(got.Subjects, tt.want.Subjects) {
				t.Errorf("Subjects = %+v, want %+v", got.Subjects, tt.want.Subjects)
			}
			if !reflect.DeepEqual(got.DeniedSpecs, tt.want.DeniedSpecs) {
				t.Errorf("DeniedSpecs = %+v, want %+v", got.DeniedSpecs, tt.want.DeniedSpecs)
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
		SpecListIDs:   []string{"uuid-jp", "uuid-en"},
		SpecListNames: []string{"Pokemon - Japanese Language Only", "Pokemon - English Language Only"},
		SubjectFilter: psacampaign.CampaignFilter{Type: "Target"},
	}
	linkedIncomplete := psacampaign.PortalCampaign{CampaignRequestID: "req-2", TargetingComplete: false}
	// Same shape as linkedComplete (a cleanly resolvable language set and
	// subject filter) except for TargetingComplete, so a case built from it
	// isolates the TargetingComplete guard: if that guard were ever bypassed,
	// this fixture would resolve and write instead of skip, unlike
	// linkedIncomplete (which also fails on empty SpecListNames and would mask
	// the bypass).
	linkedIncompleteOtherwiseValid := psacampaign.PortalCampaign{
		CampaignRequestID: "req-1", TargetingComplete: false,
		SpecListIDs:   []string{"uuid-jp", "uuid-en"},
		SpecListNames: []string{"Pokemon - Japanese Language Only", "Pokemon - English Language Only"},
		SubjectFilter: psacampaign.CampaignFilter{Type: "Target"},
	}
	linkedNoSpecList := psacampaign.PortalCampaign{
		CampaignRequestID: "req-3", TargetingComplete: true,
		SpecListIDs:   []string{},
		SpecListNames: []string{}, // no curated list -> unconverted CATEGORY campaign, §8
	}
	// The mode hole, end to end: a raw portal string that is neither Target nor
	// Exclude must never reach the database.
	linkedBadFilterType := psacampaign.PortalCampaign{
		CampaignRequestID: "req-1", TargetingComplete: true,
		SpecListIDs:   []string{"uuid-en"},
		SpecListNames: []string{"Pokemon - English Language Only"},
		SubjectFilter: psacampaign.CampaignFilter{Type: "Include"},
	}
	// The decode-time drop, end to end: the catalog explained neither id.
	linkedUnexplainedIDs := psacampaign.PortalCampaign{
		CampaignRequestID: "req-1", TargetingComplete: true,
		SpecListIDs:   []string{"uuid-unknown"},
		SpecListNames: []string{},
		SubjectFilter: psacampaign.CampaignFilter{Type: "Target"},
	}
	notLinked := psacampaign.PortalCampaign{CampaignRequestID: "req-unlinked", TargetingComplete: true}

	// The internal fleet is per-case, not a shared fixture: the unobserved-links
	// check makes the result depend on which internal campaigns exist, so a case
	// that passes only one portal campaign must also narrow the fleet or it will
	// (correctly) report the other two as missing from the portal.
	//
	// Name is populated on every row because buildBaselineCampaign now runs the
	// result through inventory.ValidateAndNormalizeCampaign, which requires it.
	allThree := []inventory.Campaign{
		{ID: "camp-1", Name: "Vintage Core", PSACampaignRequestID: "req-1"},
		{ID: "camp-2", Name: "Modern Slabs", PSACampaignRequestID: "req-2"},
		{ID: "camp-3", Name: "Legacy Category", PSACampaignRequestID: "req-3"},
	}
	onlyOne := []inventory.Campaign{{ID: "camp-1", Name: "Vintage Core", PSACampaignRequestID: "req-1"}}

	tests := []struct {
		name       string
		internal   []inventory.Campaign
		portal     []psacampaign.PortalCampaign
		updateErr  error
		wantErr    bool
		wantWrites int
		wantLangs  []string // asserted on the first write, when wantWrites > 0
	}{
		{
			name:       "writes the linked complete campaign, skips the rest, exits non-zero",
			internal:   allThree,
			portal:     []psacampaign.PortalCampaign{linkedComplete, linkedIncomplete, linkedNoSpecList, notLinked},
			wantErr:    true,
			wantWrites: 1,
			wantLangs:  []string{"english", "japanese"},
		},
		{
			// The whole point of the change: a campaign carrying both curated
			// lists is the live shape and must write cleanly, exit 0.
			name:       "both curated lists writes cleanly and exits zero",
			internal:   onlyOne,
			portal:     []psacampaign.PortalCampaign{linkedComplete},
			wantErr:    false,
			wantWrites: 1,
			wantLangs:  []string{"english", "japanese"},
		},
		{
			name:       "an update failure aborts immediately",
			internal:   onlyOne,
			portal:     []psacampaign.PortalCampaign{linkedComplete},
			updateErr:  errors.New("db down"),
			wantErr:    true,
			wantWrites: 1,
			wantLangs:  []string{"english", "japanese"},
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
			name:       "an unvalidatable subject filter type skips rather than writing",
			internal:   onlyOne,
			portal:     []psacampaign.PortalCampaign{linkedBadFilterType},
			wantErr:    true,
			wantWrites: 0,
		},
		{
			name:       "spec-list ids the catalog could not explain skip rather than writing",
			internal:   onlyOne,
			portal:     []psacampaign.PortalCampaign{linkedUnexplainedIDs},
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
			wantLangs:  []string{"english", "japanese"},
		},
		{
			// Nothing in the schema stops two campaigns from claiming the same
			// portal campaign (migration 000018 added the column with no unique
			// index). Whichever one the map kept would get the portal's real
			// targeting and the other would silently keep its stale targeting,
			// so the run must refuse before writing anything at all — note
			// wantWrites is 0 even though the portal record itself is valid.
			name: "two campaigns claiming the same portal campaign refuse the run",
			internal: []inventory.Campaign{
				{ID: "camp-1", Name: "Vintage Core", PSACampaignRequestID: "req-1"},
				{ID: "camp-1b", Name: "Vintage Core Dupe", PSACampaignRequestID: "req-1"},
			},
			portal:     []psacampaign.PortalCampaign{linkedComplete},
			wantErr:    true,
			wantWrites: 0,
		},
		{
			// The guard keys on the link, not on emptiness: unlinked campaigns
			// all share the "" request id and must not be mistaken for dupes.
			name: "multiple unlinked campaigns are not a duplicate link",
			internal: []inventory.Campaign{
				{ID: "camp-1", Name: "Vintage Core", PSACampaignRequestID: "req-1"},
				{ID: "camp-x", Name: "Unlinked A"},
				{ID: "camp-y", Name: "Unlinked B"},
			},
			portal:     []psacampaign.PortalCampaign{linkedComplete},
			wantErr:    false,
			wantWrites: 1,
			wantLangs:  []string{"english", "japanese"},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			writes := 0
			var firstWritten inventory.Campaign
			repo := &mocks.CampaignRepositoryMock{
				ListCampaignsFn: func(ctx context.Context, activeOnly bool) ([]inventory.Campaign, error) {
					return tt.internal, nil
				},
				UpdateCampaignFn: func(ctx context.Context, c *inventory.Campaign) error {
					if writes == 0 {
						firstWritten = *c
					}
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
			if tt.wantLangs != nil && !reflect.DeepEqual(firstWritten.TargetLanguages, tt.wantLangs) {
				t.Errorf("first written TargetLanguages = %v, want %v", firstWritten.TargetLanguages, tt.wantLangs)
			}
		})
	}
}
