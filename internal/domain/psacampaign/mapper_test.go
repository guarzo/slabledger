package psacampaign

import (
	"errors"
	"fmt"
	"sort"
	"strings"
	"testing"

	"github.com/guarzo/slabledger/internal/domain/inventory"
)

// stubResolver is a minimal local Resolver for the tests in this file that
// need one; internal/testutil/mocks.ResolverMock is used by other packages
// (e.g. the httpserver handlers in Task 9) that cannot import psacampaign's
// unexported test helpers. Both implement the same psacampaign.Resolver.
type stubResolver struct {
	specLists map[string][]string
	subjects  map[string]int
}

// SpecListIDs mirrors catalogResolver's all-or-nothing contract: one
// unresolvable token fails the whole call, never a partial list.
func (s stubResolver) SpecListIDs(languageTokens []string) ([]string, error) {
	var out []string
	for _, token := range languageTokens {
		ids, ok := s.specLists[token]
		if !ok {
			return nil, fmt.Errorf("%w: %q", ErrUnknownSpecList, token)
		}
		out = append(out, ids...)
	}
	return out, nil
}

func (s stubResolver) SubjectID(name string) (int, error) {
	id, ok := s.subjects[name]
	if !ok {
		return 0, ErrUnknownSubject
	}
	return id, nil
}

func englishResolver() stubResolver {
	return stubResolver{
		specLists: map[string][]string{"english": {"list-en-1"}},
		subjects:  map[string]int{"Pikachu": 90001},
	}
}

// bothLanguagesResolver is the live portal's shape: every active campaign
// carries both curated lists.
func bothLanguagesResolver() stubResolver {
	return stubResolver{
		specLists: map[string][]string{"english": {"list-en-1"}, "japanese": {"list-ja-1"}},
		subjects:  map[string]int{"Pikachu": 90001},
	}
}

func TestTranslateToDiff_ScalarFields(t *testing.T) {
	internal := inventory.Campaign{
		BuyTermsCLPct:      0.75, // 75%
		DailySpendCapCents: 400000,
		GradeRange:         "9-10",
		YearRange:          "2020-2024",
		PriceRange:         "100-3000", // dollars
		CLConfidence:       "3-4",
	}
	portal := PortalCampaign{
		BuyPercentClv:    70,
		DailyBudgetCents: 300000,
		BuyBox: CampaignBuyBox{
			GradeMin: "10", GradeMax: "10", YearMin: 2002, YearMax: 2003,
			PriceMinCents: 50000, PriceMaxCents: 200000, ClvConfidenceMin: 2,
		},
	}
	diff, err := TranslateToDiff(internal, portal, englishResolver())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	got := map[string]FieldChange{}
	for _, c := range diff.Changes {
		got[c.Field] = c
	}
	if c := got["bidPercentage"]; c.Old != "70" || c.New != "75" {
		t.Errorf("bidPercentage = %+v, want old 70 new 75", c)
	}
	if c := got["gradeMinimum"]; c.New != "9" {
		t.Errorf("gradeMinimum new = %q, want 9", c.New)
	}
	if c := got["priceMaximum"]; c.New != "3000" {
		t.Errorf("priceMaximum new = %q, want 3000 (dollars)", c.New)
	}
	if _, ok := got["yearMinimum"]; !ok {
		t.Error("expected yearMinimum change (2002 -> 2020)")
	}
}

func baseCreateCampaign() inventory.Campaign {
	return inventory.Campaign{
		Name: "Modern 10s", BuyTermsCLPct: 0.72, DailySpendCapCents: 300000,
		GradeRange: "10", YearRange: "2024-2026", PriceRange: "500-3000",
		CLConfidence: "3-4", PSASourcingFeeCents: 300,
		TargetLanguages: []string{"english"}, SubjectFilterMode: "Target",
	}
}

func TestTranslateToCreate(t *testing.T) {
	base := baseCreateCampaign()
	tests := []struct {
		name    string
		mutate  func(c *inventory.Campaign)
		wantErr string
		check   func(t *testing.T, fd CampaignFormData)
	}{
		{
			name:   "full mapping, born paused",
			mutate: func(c *inventory.Campaign) {},
			check: func(t *testing.T, fd CampaignFormData) {
				if fd.IsActive {
					t.Fatal("create must be born paused (isActive=false)")
				}
				if fd.CampaignName != "Modern 10s" || fd.CampaignType != "SPEC_LIST" || fd.Category != "" {
					t.Fatalf("identity fields wrong: %+v", fd)
				}
				if fd.BidPercentage != 72 {
					t.Fatalf("BidPercentage = %d, want 72", fd.BidPercentage)
				}
				if fd.DailyBudget != 3000 {
					t.Fatalf("DailyBudget = %d, want 3000 (whole USD)", fd.DailyBudget)
				}
				if fd.FlatFee != 3 {
					t.Fatalf("FlatFee = %d, want 3 (PSASourcingFeeCents/100)", fd.FlatFee)
				}
				if fd.DailySpecLimit != 2 {
					t.Fatalf("DailySpecLimit = %d, want default 2", fd.DailySpecLimit)
				}
				if fd.GradeMinimum != "10" || fd.GradeMaximum != "10" {
					t.Fatalf("grades = %q/%q, want \"10\"/\"10\"", fd.GradeMinimum, fd.GradeMaximum)
				}
				if fd.YearMinimum != 2024 || fd.YearMaximum != 2026 {
					t.Fatalf("years = %d/%d", fd.YearMinimum, fd.YearMaximum)
				}
				if fd.PriceMinimum != 500 || fd.PriceMaximum != 3000 {
					t.Fatalf("prices = %d/%d", fd.PriceMinimum, fd.PriceMaximum)
				}
				if fd.CardLadderConfidenceMinimum != 3 {
					t.Fatalf("clConf = %d, want 3", fd.CardLadderConfidenceMinimum)
				}
				if fd.PublisherFilterType != "Target" || fd.SubjectFilterType != "Target" {
					t.Fatalf("filter types wrong: %+v", fd)
				}
				if len(fd.PrepackagedSpecListIDs) != 1 || fd.PrepackagedSpecListIDs[0] != "list-en-1" {
					t.Fatalf("PrepackagedSpecListIDs = %+v, want [list-en-1]", fd.PrepackagedSpecListIDs)
				}
				// Wire format requires [] not null for the list fields.
				if fd.SelectedPublishers == nil || fd.SelectedSubjects == nil || fd.DeniedSpecs == nil {
					t.Fatal("list fields must be non-nil empty slices (JSON [] not null)")
				}
			},
		},
		{
			name:   "decimal cl confidence truncates",
			mutate: func(c *inventory.Campaign) { c.CLConfidence = "2.5-4" },
			check: func(t *testing.T, fd CampaignFormData) {
				if fd.CardLadderConfidenceMinimum != 2 {
					t.Fatalf("clConf = %d, want 2 (truncated from 2.5)", fd.CardLadderConfidenceMinimum)
				}
			},
		},
		{
			name: "non-multiple-of-100 cents round to nearest USD",
			mutate: func(c *inventory.Campaign) {
				c.PSASourcingFeeCents = 350   // $3.50 -> $4
				c.DailySpendCapCents = 299900 // $2999.00 -> $2999
			},
			check: func(t *testing.T, fd CampaignFormData) {
				if fd.FlatFee != 4 {
					t.Fatalf("FlatFee = %d, want 4 (350c rounds up)", fd.FlatFee)
				}
				if fd.DailyBudget != 2999 {
					t.Fatalf("DailyBudget = %d, want 2999", fd.DailyBudget)
				}
			},
		},
		{name: "empty grade range", mutate: func(c *inventory.Campaign) { c.GradeRange = "" }, wantErr: "grade range"},
		{name: "empty year range", mutate: func(c *inventory.Campaign) { c.YearRange = "" }, wantErr: "year range"},
		{name: "non-numeric year", mutate: func(c *inventory.Campaign) { c.YearRange = "vintage-2020" }, wantErr: "year range"},
		{name: "empty price range", mutate: func(c *inventory.Campaign) { c.PriceRange = "" }, wantErr: "price range"},
		{name: "empty CL confidence", mutate: func(c *inventory.Campaign) { c.CLConfidence = "" }, wantErr: "cl confidence"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c := base
			tt.mutate(&c)
			fd, err := TranslateToCreate(c, englishResolver())
			if tt.wantErr != "" {
				if err == nil || !strings.Contains(err.Error(), tt.wantErr) {
					t.Fatalf("err = %v, want containing %q", err, tt.wantErr)
				}
				return
			}
			if err != nil {
				t.Fatalf("TranslateToCreate: %v", err)
			}
			tt.check(t, fd)
		})
	}
}

func TestTranslateToCreate_SpecListAndSubjects(t *testing.T) {
	tests := []struct {
		name      string
		mutate    func(c *inventory.Campaign)
		r         Resolver
		wantErr   string
		wantErrIs error
		check     func(t *testing.T, fd CampaignFormData)
	}{
		{
			name:   "spec-list type and resolved subject",
			mutate: func(c *inventory.Campaign) { c.Subjects = []inventory.TargetSubject{{Name: "Pikachu"}} },
			r:      englishResolver(),
			check: func(t *testing.T, fd CampaignFormData) {
				if fd.CampaignType != "SPEC_LIST" || fd.Category != "" {
					t.Fatalf("CampaignType/Category = %q/%q, want SPEC_LIST/\"\"", fd.CampaignType, fd.Category)
				}
				if len(fd.PrepackagedSpecListIDs) != 1 || fd.PrepackagedSpecListIDs[0] != "list-en-1" {
					t.Fatalf("PrepackagedSpecListIDs = %+v, want [list-en-1]", fd.PrepackagedSpecListIDs)
				}
				if len(fd.SelectedSubjects) != 1 || fd.SelectedSubjects[0] != (SubjectRef{ID: 90001, Name: "Pikachu"}) {
					t.Fatalf("SelectedSubjects = %+v, want [{90001 Pikachu}]", fd.SelectedSubjects)
				}
			},
		},
		{
			name:   "portal-sourced subject id passes through verbatim",
			mutate: func(c *inventory.Campaign) { c.Subjects = []inventory.TargetSubject{{ID: 4807, Name: "Charizard"}} },
			r:      englishResolver(),
			check: func(t *testing.T, fd CampaignFormData) {
				if len(fd.SelectedSubjects) != 1 || fd.SelectedSubjects[0] != (SubjectRef{ID: 4807, Name: "Charizard"}) {
					t.Fatalf("SelectedSubjects = %+v, want [{4807 Charizard}] unresolved", fd.SelectedSubjects)
				}
			},
		},
		{
			name:    "empty target language fails",
			mutate:  func(c *inventory.Campaign) { c.TargetLanguages = nil },
			r:       englishResolver(),
			wantErr: "target language",
		},
		{
			name:      "unmapped language token fails",
			mutate:    func(c *inventory.Campaign) { c.TargetLanguages = []string{"korean"} },
			r:         englishResolver(),
			wantErr:   "resolve spec list",
			wantErrIs: ErrUnknownSpecList,
		},
		{
			name:      "unresolvable subject name fails naming the subject",
			mutate:    func(c *inventory.Campaign) { c.Subjects = []inventory.TargetSubject{{Name: "Mewtwo"}} },
			r:         englishResolver(),
			wantErr:   "Mewtwo",
			wantErrIs: ErrUnknownSubject,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c := baseCreateCampaign()
			tt.mutate(&c)
			fd, err := TranslateToCreate(c, tt.r)
			if tt.wantErr != "" {
				if err == nil || !strings.Contains(err.Error(), tt.wantErr) {
					t.Fatalf("err = %v, want containing %q", err, tt.wantErr)
				}
				if tt.wantErrIs != nil && !errors.Is(err, tt.wantErrIs) {
					t.Fatalf("errors.Is(err, %v) = false, err = %v", tt.wantErrIs, err)
				}
				return
			}
			if err != nil {
				t.Fatalf("TranslateToCreate: %v", err)
			}
			tt.check(t, fd)
		})
	}
}

func TestTranslateToDiff_ListAxes(t *testing.T) {
	r := englishResolver()
	internal := inventory.Campaign{
		BuyTermsCLPct: 0.75, DailySpendCapCents: 400000,
		GradeRange: "9-10", YearRange: "2020-2024", PriceRange: "100-3000", CLConfidence: "3-4",
		TargetLanguages: []string{"english"}, SubjectFilterMode: "Exclude",
		Subjects:    []inventory.TargetSubject{{ID: 22210, Name: "Machamp"}, {ID: 4807, Name: "Charizard"}},
		DeniedSpecs: []inventory.TargetSubject{{ID: 22301, Name: "Charizard EX"}},
	}
	portal := PortalCampaign{
		BuyPercentClv: 75, DailyBudgetCents: 400000,
		BuyBox: CampaignBuyBox{
			GradeMin: "9", GradeMax: "10", YearMin: 2020, YearMax: 2024,
			PriceMinCents: 10000, PriceMaxCents: 300000, ClvConfidenceMin: 3,
		},
		SubjectFilter: CampaignFilter{
			Type: "Target",
			// Same two subjects, REVERSED order plus one difference: this
			// asserts order-insensitivity would suppress a spurious change
			// while the actual polarity change still fires.
			Subjects: []SubjectRef{{ID: 4807, Name: "Charizard"}, {ID: 22210, Name: "Machamp"}},
		},
		SpecListIDs: []string{"list-en-1"},
		DeniedSpecs: []SubjectRef{{ID: 22301, Name: "Charizard EX"}},
	}

	diff, err := TranslateToDiff(internal, portal, r)
	if err != nil {
		t.Fatalf("TranslateToDiff: %v", err)
	}
	got := map[string]FieldChange{}
	for _, c := range diff.Changes {
		got[c.Field] = c
	}

	if _, ok := got["selectedSubjects"]; ok {
		t.Errorf("selectedSubjects should NOT appear: identical subject sets in different order must not diff")
	}
	if c, ok := got["subjectFilterType"]; !ok || c.Old != "Target" || c.New != "Exclude" {
		t.Errorf("subjectFilterType = %+v, want Target -> Exclude", c)
	}
	if _, ok := got["deniedSpecs"]; ok {
		t.Errorf("deniedSpecs should NOT appear: identical denied lists must not diff")
	}
	if _, ok := got["prepackagedSpecListIds"]; ok {
		t.Errorf("prepackagedSpecListIds should NOT appear: identical single-entry lists must not diff")
	}

	// Now change one subject's name and assert the diff DOES fire and carries
	// a typed Value (not just a rendered string), for push.go to consume.
	internal.Subjects = []inventory.TargetSubject{{ID: 22210, Name: "Machamp"}, {ID: 90001, Name: "Pikachu"}}
	diff2, err := TranslateToDiff(internal, portal, r)
	if err != nil {
		t.Fatalf("TranslateToDiff (2): %v", err)
	}
	var found *FieldChange
	for i := range diff2.Changes {
		if diff2.Changes[i].Field == "selectedSubjects" {
			found = &diff2.Changes[i]
		}
	}
	if found == nil {
		t.Fatal("expected a selectedSubjects change after swapping Charizard for Pikachu")
	}
	refs, ok := found.Value.([]SubjectRef)
	if !ok || len(refs) != 2 {
		t.Fatalf("Value = %#v, want []SubjectRef of length 2", found.Value)
	}

	// Now make the portal's spec-list ids stale relative to the resolver's
	// mapping for the campaign's TargetLanguage, and assert prepackagedSpecListIds
	// DOES fire, carrying the resolved (not portal-stale) list as its typed Value.
	portal.SpecListIDs = []string{"stale-list"}
	diff3, err := TranslateToDiff(internal, portal, r)
	if err != nil {
		t.Fatalf("TranslateToDiff (3): %v", err)
	}
	var specListChange *FieldChange
	for i := range diff3.Changes {
		if diff3.Changes[i].Field == "prepackagedSpecListIds" {
			specListChange = &diff3.Changes[i]
		}
	}
	if specListChange == nil {
		t.Fatal("expected a prepackagedSpecListIds change when portal.SpecListIDs is stale relative to the resolver")
	}
	specListIDs, ok := specListChange.Value.([]string)
	if !ok || len(specListIDs) != 1 || specListIDs[0] != "list-en-1" {
		t.Fatalf("prepackagedSpecListIds Value = %#v, want []string{\"list-en-1\"}", specListChange.Value)
	}
	portal.SpecListIDs = []string{"list-en-1"} // restore before the next assertion

	// Now change a denied spec and assert deniedSpecs DOES fire.
	internal.DeniedSpecs = []inventory.TargetSubject{{ID: 22301, Name: "Charizard EX"}, {ID: 4807, Name: "Charizard"}}
	diff4, err := TranslateToDiff(internal, portal, r)
	if err != nil {
		t.Fatalf("TranslateToDiff (4): %v", err)
	}
	var deniedChange *FieldChange
	for i := range diff4.Changes {
		if diff4.Changes[i].Field == "deniedSpecs" {
			deniedChange = &diff4.Changes[i]
		}
	}
	if deniedChange == nil {
		t.Fatal("expected a deniedSpecs change after adding Charizard to the denied list")
	}
	deniedRefs, ok := deniedChange.Value.([]SubjectRef)
	if !ok || len(deniedRefs) != 2 {
		t.Fatalf("deniedSpecs Value = %#v, want []SubjectRef of length 2", deniedChange.Value)
	}
}

// TestTranslateToDiff_EmptyLocalSubjectsProposesClearingPortalList pins the
// (intentional, per design doc) behavior that when SlabLedger's Subjects list
// is empty but the portal campaign still has a non-empty subject list, the
// diff proposes clearing it: SlabLedger is authoritative, and the proposal is
// human-approved via the push queue before anything reaches the portal.
func TestTranslateToDiff_EmptyLocalSubjectsProposesClearingPortalList(t *testing.T) {
	r := englishResolver()
	internal := inventory.Campaign{
		BuyTermsCLPct: 0.75, DailySpendCapCents: 400000,
		GradeRange: "9-10", YearRange: "2020-2024", PriceRange: "100-3000", CLConfidence: "3-4",
		TargetLanguages: []string{"english"}, SubjectFilterMode: "Target",
		Subjects: nil, // no locally-tracked subjects
	}
	portal := PortalCampaign{
		BuyPercentClv: 75, DailyBudgetCents: 400000,
		BuyBox: CampaignBuyBox{
			GradeMin: "9", GradeMax: "10", YearMin: 2020, YearMax: 2024,
			PriceMinCents: 10000, PriceMaxCents: 300000, ClvConfidenceMin: 3,
		},
		SubjectFilter: CampaignFilter{
			Type:     "Target",
			Subjects: []SubjectRef{{ID: 22210, Name: "Machamp"}},
		},
		SpecListIDs: []string{"list-en-1"},
	}

	diff, err := TranslateToDiff(internal, portal, r)
	if err != nil {
		t.Fatalf("TranslateToDiff: %v", err)
	}
	var found *FieldChange
	for i := range diff.Changes {
		if diff.Changes[i].Field == "selectedSubjects" {
			found = &diff.Changes[i]
		}
	}
	if found == nil {
		t.Fatal("expected a selectedSubjects change proposing to clear the portal's non-empty subject list")
	}
	refs, ok := found.Value.([]SubjectRef)
	if !ok || len(refs) != 0 {
		t.Fatalf("selectedSubjects Value = %#v, want an empty []SubjectRef", found.Value)
	}
}

// TestTranslateToDiff_ErrorSurface pins TranslateToDiff's own error paths —
// distinct from TranslateToCreate's, since mapper.go:86 (spec-list resolve)
// and the toSubjectRefs propagations at :67/:74 are separate fmt.Errorf call
// sites on the diff side. Every other TranslateToDiff test asserts err == nil,
// so without this, a %w -> %v regression here would silently degrade Task 9's
// 4xx classification to a 500 while the rest of the suite stayed green.
func TestTranslateToDiff_ErrorSurface(t *testing.T) {
	base := func() (inventory.Campaign, PortalCampaign) {
		internal := inventory.Campaign{
			BuyTermsCLPct: 0.75, DailySpendCapCents: 400000,
			GradeRange: "9-10", YearRange: "2020-2024", PriceRange: "100-3000", CLConfidence: "3-4",
			TargetLanguages: []string{"english"}, SubjectFilterMode: "Target",
		}
		portal := PortalCampaign{
			BuyPercentClv: 75, DailyBudgetCents: 400000,
			BuyBox: CampaignBuyBox{
				GradeMin: "9", GradeMax: "10", YearMin: 2020, YearMax: 2024,
				PriceMinCents: 10000, PriceMaxCents: 300000, ClvConfidenceMin: 3,
			},
			SubjectFilter: CampaignFilter{Type: "Target"},
		}
		return internal, portal
	}

	t.Run("unmapped language token fails", func(t *testing.T) {
		internal, portal := base()
		internal.TargetLanguages = []string{"korean"}
		_, err := TranslateToDiff(internal, portal, englishResolver())
		if err == nil || !strings.Contains(err.Error(), "resolve spec list") {
			t.Fatalf("err = %v, want containing %q", err, "resolve spec list")
		}
		if !errors.Is(err, ErrUnknownSpecList) {
			t.Fatalf("errors.Is(err, ErrUnknownSpecList) = false, err = %v", err)
		}
	})

	t.Run("unresolvable subject name fails naming the subject", func(t *testing.T) {
		internal, portal := base()
		internal.Subjects = []inventory.TargetSubject{{Name: "Mewtwo"}}
		_, err := TranslateToDiff(internal, portal, englishResolver())
		if err == nil || !strings.Contains(err.Error(), "Mewtwo") {
			t.Fatalf("err = %v, want containing %q", err, "Mewtwo")
		}
		if !errors.Is(err, ErrUnknownSubject) {
			t.Fatalf("errors.Is(err, ErrUnknownSubject) = false, err = %v", err)
		}
	})
}

func TestTranslateToDiff_EmptyTargetLanguagesSkipsSpecListAxis(t *testing.T) {
	r := englishResolver()
	internal := inventory.Campaign{
		BuyTermsCLPct: 0.75, DailySpendCapCents: 400000,
		GradeRange: "9-10", YearRange: "2020-2024", PriceRange: "100-3000", CLConfidence: "3-4",
		TargetLanguages: nil, SubjectFilterMode: "Target",
	}
	portal := PortalCampaign{
		BuyPercentClv: 75, DailyBudgetCents: 400000,
		BuyBox: CampaignBuyBox{
			GradeMin: "9", GradeMax: "10", YearMin: 2020, YearMax: 2024,
			PriceMinCents: 10000, PriceMaxCents: 300000, ClvConfidenceMin: 3,
		},
		SubjectFilter: CampaignFilter{Type: "Target"},
		SpecListIDs:   []string{"stale-list"},
	}
	diff, err := TranslateToDiff(internal, portal, r)
	if err != nil {
		t.Fatalf("TranslateToDiff: %v (empty TargetLanguages must not error the whole diff)", err)
	}
	for _, c := range diff.Changes {
		if c.Field == "prepackagedSpecListIds" {
			t.Fatalf("did not expect a prepackagedSpecListIds change with an unset TargetLanguages: %+v", c)
		}
	}
}

// TestTranslateToDiff_MultiLanguageSpecLists is the regression this whole
// change exists for. Before it, a campaign carrying both curated lists
// translated to a single-element prepackagedSpecListIds, so the first push
// would have DROPPED one curated list from every live campaign — silently
// changing what six money-spending campaigns buy.
func TestTranslateToDiff_MultiLanguageSpecLists(t *testing.T) {
	r := bothLanguagesResolver()
	base := func() (inventory.Campaign, PortalCampaign) {
		internal := inventory.Campaign{
			BuyTermsCLPct: 0.75, DailySpendCapCents: 400000,
			GradeRange: "9-10", YearRange: "2020-2024", PriceRange: "100-3000", CLConfidence: "3-4",
			TargetLanguages: []string{"english", "japanese"}, SubjectFilterMode: "Target",
		}
		portal := PortalCampaign{
			BuyPercentClv: 75, DailyBudgetCents: 400000,
			BuyBox: CampaignBuyBox{
				GradeMin: "9", GradeMax: "10", YearMin: 2020, YearMax: 2024,
				PriceMinCents: 10000, PriceMaxCents: 300000, ClvConfidenceMin: 3,
			},
			SubjectFilter: CampaignFilter{Type: "Target"},
		}
		return internal, portal
	}

	specListChange := func(t *testing.T, d ProposedDiff) *FieldChange {
		t.Helper()
		for i := range d.Changes {
			if d.Changes[i].Field == "prepackagedSpecListIds" {
				return &d.Changes[i]
			}
		}
		return nil
	}

	t.Run("portal already carries both lists, reversed, produces no diff", func(t *testing.T) {
		internal, portal := base()
		// Reversed relative to the resolver's token order: renderStringList's
		// canonical (sorted) rendering must absorb that for a two-element list.
		portal.SpecListIDs = []string{"list-ja-1", "list-en-1"}
		d, err := TranslateToDiff(internal, portal, r)
		if err != nil {
			t.Fatalf("TranslateToDiff: %v", err)
		}
		if c := specListChange(t, d); c != nil {
			t.Fatalf("unexpected prepackagedSpecListIds change for the same two lists in a different order: %+v", c)
		}
	})

	t.Run("portal missing one list proposes both, not one", func(t *testing.T) {
		internal, portal := base()
		portal.SpecListIDs = []string{"list-en-1"}
		d, err := TranslateToDiff(internal, portal, r)
		if err != nil {
			t.Fatalf("TranslateToDiff: %v", err)
		}
		c := specListChange(t, d)
		if c == nil {
			t.Fatal("expected a prepackagedSpecListIds change when the portal is missing the japanese list")
		}
		ids, ok := c.Value.([]string)
		if !ok {
			t.Fatalf("Value = %#v, want []string", c.Value)
		}
		got := append([]string(nil), ids...)
		sort.Strings(got)
		if len(got) != 2 || got[0] != "list-en-1" || got[1] != "list-ja-1" {
			t.Fatalf("prepackagedSpecListIds Value = %v, want both list-en-1 and list-ja-1", got)
		}
	})

	t.Run("one unresolvable token fails the whole call, never a partial list", func(t *testing.T) {
		internal, portal := base()
		internal.TargetLanguages = []string{"english", "korean"}
		_, err := TranslateToDiff(internal, portal, r)
		if err == nil {
			t.Fatal("expected an error: a partial spec list would narrow a live campaign")
		}
		if !errors.Is(err, ErrUnknownSpecList) {
			t.Fatalf("errors.Is(err, ErrUnknownSpecList) = false, err = %v", err)
		}
		if !strings.Contains(err.Error(), "korean") {
			t.Fatalf("err = %v, want it to name the offending token %q", err, "korean")
		}
	})
}

// TestToSubjectRefs_LegacySentinelRefused pins locked decision 6: migration
// 000023's legacy backfill is marked with a distinct sentinel id, and
// translation refuses it outright. The genuine operator-typed id-0 path — a
// name the operator entered that has never been reconciled with the portal —
// must keep resolving, since those two cases were previously
// indistinguishable and re-resolving a legacy subject would swap live
// 4xxx/8xxx portal ids for current-generation 22xxx ids.
func TestToSubjectRefs_LegacySentinelRefused(t *testing.T) {
	tests := []struct {
		name      string
		subjects  []inventory.TargetSubject
		wantIDs   []int
		wantErrIs error
		wantErrIn string
	}{
		{
			name:     "portal-sourced id passes through verbatim",
			subjects: []inventory.TargetSubject{{ID: 4807, Name: "Charizard"}},
			wantIDs:  []int{4807},
		},
		{
			name:     "operator-typed id 0 still resolves by name",
			subjects: []inventory.TargetSubject{{Name: "Pikachu"}},
			wantIDs:  []int{90001},
		},
		{
			name:      "legacy sentinel is refused, naming the subject",
			subjects:  []inventory.TargetSubject{{ID: inventory.LegacyUnreconciledSubjectID, Name: "Charizard"}},
			wantErrIs: ErrLegacySubjectsUnreconciled,
			wantErrIn: "Charizard",
		},
		{
			name: "one sentinel poisons an otherwise resolvable list",
			subjects: []inventory.TargetSubject{
				{ID: 4807, Name: "Charizard"},
				{ID: inventory.LegacyUnreconciledSubjectID, Name: "Machamp"},
			},
			wantErrIs: ErrLegacySubjectsUnreconciled,
			wantErrIn: "Machamp",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			refs, err := toSubjectRefs(tt.subjects, englishResolver())
			if tt.wantErrIs != nil {
				if !errors.Is(err, tt.wantErrIs) {
					t.Fatalf("errors.Is(err, %v) = false, err = %v", tt.wantErrIs, err)
				}
				if !strings.Contains(err.Error(), tt.wantErrIn) {
					t.Fatalf("err = %v, want it to name %q", err, tt.wantErrIn)
				}
				if !strings.Contains(err.Error(), "baseline") {
					t.Fatalf("err = %v, want it to tell the operator to run the baseline pull", err)
				}
				if refs != nil {
					t.Fatalf("refs = %+v, want nil on error", refs)
				}
				return
			}
			if err != nil {
				t.Fatalf("toSubjectRefs: %v", err)
			}
			if len(refs) != len(tt.wantIDs) {
				t.Fatalf("refs = %+v, want %d entries", refs, len(tt.wantIDs))
			}
			for i, want := range tt.wantIDs {
				if refs[i].ID != want {
					t.Fatalf("refs[%d].ID = %d, want %d", i, refs[i].ID, want)
				}
			}
		})
	}
}

// TestTranslateToCreate_LegacySentinelRefused covers the create side of the
// same refusal — TranslateToCreate calls toSubjectRefs for both Subjects and
// DeniedSpecs, and a create pushed with re-resolved legacy ids is just as
// wrong as an update.
func TestTranslateToCreate_LegacySentinelRefused(t *testing.T) {
	c := baseCreateCampaign()
	c.DeniedSpecs = []inventory.TargetSubject{{ID: inventory.LegacyUnreconciledSubjectID, Name: "Charizard EX"}}
	_, err := TranslateToCreate(c, englishResolver())
	if !errors.Is(err, ErrLegacySubjectsUnreconciled) {
		t.Fatalf("errors.Is(err, ErrLegacySubjectsUnreconciled) = false, err = %v", err)
	}
}
