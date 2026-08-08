package scheduler

import (
	"context"
	"encoding/json"
	"errors"
	"sort"
	"strconv"
	"testing"

	"github.com/guarzo/slabledger/internal/adapters/clients/dh"
	"github.com/guarzo/slabledger/internal/domain/demand"
	"github.com/guarzo/slabledger/internal/testutil/mocks"
)

// attributionRepo is a DemandRepositoryMock wired as a tiny in-memory card
// cache, so the attribution step's read-modify-write against GetCardCache /
// UpsertCardCache exercises real merge behavior rather than canned returns.
type attributionRepo struct {
	*mocks.DemandRepositoryMock
	cards      map[string]demand.CardCache
	characters map[string]demand.CharacterCache
}

func newAttributionRepo(seed ...demand.CardCache) *attributionRepo {
	r := &attributionRepo{
		DemandRepositoryMock: &mocks.DemandRepositoryMock{},
		cards:                make(map[string]demand.CardCache, len(seed)),
		characters:           make(map[string]demand.CharacterCache),
	}
	for _, c := range seed {
		r.cards[c.CardID] = c
	}
	r.UpsertCardCacheFn = func(_ context.Context, row demand.CardCache) error {
		// Mirror the COALESCE in the real upsert: a nil character never clears
		// an attribution that is already stored.
		if row.CharacterName == nil {
			if prev, ok := r.cards[row.CardID]; ok {
				row.CharacterName = prev.CharacterName
			}
		}
		r.cards[row.CardID] = row
		return nil
	}
	r.GetCardCacheFn = func(_ context.Context, cardID, _ string) (*demand.CardCache, error) {
		row, ok := r.cards[cardID]
		if !ok {
			return nil, nil
		}
		return &row, nil
	}
	r.ListCardsMissingCharacterFn = func(_ context.Context, _ string, limit int) ([]string, error) {
		var ids []string
		for id, row := range r.cards {
			if row.CharacterName == nil {
				ids = append(ids, id)
			}
		}
		sort.Strings(ids)
		if len(ids) > limit {
			ids = ids[:limit]
		}
		return ids, nil
	}
	r.ListCardCharacterQualityFn = func(_ context.Context, _ string) ([]demand.CardCharacterQuality, error) {
		var out []demand.CardCharacterQuality
		for _, row := range r.cards {
			if row.CharacterName == nil || row.DemandDataQuality == nil {
				continue
			}
			out = append(out, demand.CardCharacterQuality{
				CharacterName: *row.CharacterName,
				DataQuality:   *row.DemandDataQuality,
			})
		}
		return out, nil
	}
	r.UpsertCharacterCacheFn = func(_ context.Context, row demand.CharacterCache) error {
		r.characters[row.Character] = row
		return nil
	}
	return r
}

// blobQuality extracts data_quality from a character cache row's demand blob.
// Reports false when the key is omitted. A missing character row is a test
// failure rather than a false return, so "the key is absent" assertions cannot
// pass just because the character was never upserted at all.
func blobQuality(t *testing.T, repo *attributionRepo, character string) (string, bool) {
	t.Helper()
	row, ok := repo.characters[character]
	if !ok {
		t.Fatalf("no character cache row upserted for %q", character)
	}
	if row.DemandJSON == nil {
		return "", false
	}
	var blob struct {
		DataQuality string `json:"data_quality"`
	}
	if err := json.Unmarshal([]byte(*row.DemandJSON), &blob); err != nil {
		t.Fatalf("unmarshal demand blob for %q: %v", character, err)
	}
	return blob.DataQuality, blob.DataQuality != ""
}

// analyticsClientFor builds a client that attributes cards per cardToCharacter
// and reports each card's demand quality per cardToQuality. extraCharacters
// appear on the leaderboard without owning any of our cards.
func analyticsClientFor(
	cardToCharacter map[int]string,
	cardToQuality map[int]string,
	extraCharacters ...string,
) *fakeAnalyticsClient {
	return &fakeAnalyticsClient{
		available: true,
		topCharactersFn: func(_ context.Context, _ int, era string) (*dh.TopCharactersResponse, error) {
			if era != "" {
				return &dh.TopCharactersResponse{}, nil
			}
			seen := map[string]struct{}{}
			var entries []dh.CharacterDemandEntry
			add := func(name string) {
				if _, dup := seen[name]; dup {
					return
				}
				seen[name] = struct{}{}
				entries = append(entries, dh.CharacterDemandEntry{CharacterName: name, CardCount: 1})
			}
			for _, name := range cardToCharacter {
				add(name)
			}
			for _, name := range extraCharacters {
				add(name)
			}
			sort.Slice(entries, func(i, j int) bool {
				return entries[i].CharacterName < entries[j].CharacterName
			})
			return &dh.TopCharactersResponse{
				ComputedAt:      "2026-08-08T00:00:00Z",
				CharacterDemand: entries,
			}, nil
		},
		characterDemandFn: func(_ context.Context, cardIDs []int, _ string, _ bool) (*dh.CharacterDemandResponse, error) {
			if len(cardIDs) != 1 {
				return nil, errors.New("attribution must seed exactly one card id")
			}
			name, ok := cardToCharacter[cardIDs[0]]
			if !ok {
				return &dh.CharacterDemandResponse{}, nil
			}
			return &dh.CharacterDemandResponse{
				CharacterDemand: []dh.CharacterDemandEntry{{CharacterName: name, CardCount: 1}},
			}, nil
		},
		demandSignalsFn: func(_ context.Context, cardIDs []int, _ string) (*dh.DemandSignalsResponse, error) {
			resp := &dh.DemandSignalsResponse{}
			for _, id := range cardIDs {
				quality, ok := cardToQuality[id]
				if !ok {
					continue
				}
				resp.DemandSignals = append(resp.DemandSignals, dh.DemandSignal{
					CardID:      id,
					DemandScore: 50,
					DataQuality: quality,
				})
			}
			return resp, nil
		},
	}
}

func TestDHAnalyticsRefresh_CharacterDataQuality(t *testing.T) {
	tests := []struct {
		name            string
		cardToCharacter map[int]string
		cardToQuality   map[int]string
		extraCharacters []string          // on the leaderboard, but own none of our cards
		wantQuality     map[string]string // character -> expected blob data_quality
		wantAbsent      []string          // characters whose blob must omit data_quality
	}{
		{
			name:            "a single full card makes its character full",
			cardToCharacter: map[int]string{1: "Charizard"},
			cardToQuality:   map[int]string{1: demand.QualityFull},
			wantQuality:     map[string]string{"Charizard": demand.QualityFull},
		},
		{
			name:            "any full card among proxies wins",
			cardToCharacter: map[int]string{1: "Charizard", 2: "Charizard", 3: "Charizard"},
			cardToQuality: map[int]string{
				1: demand.QualityProxy,
				2: demand.QualityFull,
				3: demand.QualityProxy,
			},
			wantQuality: map[string]string{"Charizard": demand.QualityFull},
		},
		{
			name:            "all-proxy cards leave the character proxy",
			cardToCharacter: map[int]string{1: "Mewtwo", 2: "Mewtwo"},
			cardToQuality:   map[int]string{1: demand.QualityProxy, 2: demand.QualityProxy},
			wantQuality:     map[string]string{"Mewtwo": demand.QualityProxy},
		},
		{
			name:            "characters are scored independently",
			cardToCharacter: map[int]string{1: "Charizard", 2: "Mewtwo"},
			cardToQuality:   map[int]string{1: demand.QualityFull, 2: demand.QualityProxy},
			wantQuality: map[string]string{
				"Charizard": demand.QualityFull,
				"Mewtwo":    demand.QualityProxy,
			},
		},
		{
			name:            "a character owning none of our cards gets no quality",
			cardToCharacter: map[int]string{1: "Charizard"},
			cardToQuality:   map[int]string{1: demand.QualityFull},
			extraCharacters: []string{"Zapdos"},
			wantQuality:     map[string]string{"Charizard": demand.QualityFull},
			wantAbsent:      []string{"Zapdos"},
		},
		{
			name:            "an attributed card with no demand signal leaves no quality",
			cardToCharacter: map[int]string{1: "Snorlax"},
			cardToQuality:   map[int]string{},
			wantQuality:     map[string]string{},
			wantAbsent:      []string{"Snorlax"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			seedIDs := []int{1, 2, 3}
			client := analyticsClientFor(tt.cardToCharacter, tt.cardToQuality, tt.extraCharacters...)
			repo := newAttributionRepo()
			sched := NewDHAnalyticsRefreshScheduler(
				client, repo, &fakeCardLister{ids: seedIDs}, mocks.NewMockLogger(), newTestCfg())

			sched.refresh(context.Background())

			for character, want := range tt.wantQuality {
				got, ok := blobQuality(t, repo, character)
				if !ok {
					t.Fatalf("character %q has no data_quality in its demand blob; want %q", character, want)
				}
				if got != want {
					t.Errorf("character %q data_quality = %q; want %q", character, got, want)
				}
			}
			for _, character := range tt.wantAbsent {
				if got, ok := blobQuality(t, repo, character); ok {
					t.Errorf("character %q data_quality = %q; want the key omitted", character, got)
				}
			}
		})
	}
}

func TestDHAnalyticsRefresh_AttributionIsCachedAcrossRuns(t *testing.T) {
	cardToCharacter := map[int]string{1: "Charizard"}
	cardToQuality := map[int]string{1: demand.QualityFull}
	client := analyticsClientFor(cardToCharacter, cardToQuality)
	repo := newAttributionRepo()
	sched := NewDHAnalyticsRefreshScheduler(
		client, repo, &fakeCardLister{ids: []int{1}}, mocks.NewMockLogger(), newTestCfg())

	sched.refresh(context.Background())
	afterFirst := client.characterDemandCalls
	if afterFirst != 1 {
		t.Fatalf("first run made %d character_demand calls; want 1", afterFirst)
	}
	if got, _ := blobQuality(t, repo, "Charizard"); got != demand.QualityFull {
		t.Fatalf("after first run data_quality = %q; want %q", got, demand.QualityFull)
	}

	// The second run rewrites every card row (with no character on the way in).
	// The attribution must survive that, so no further lookups are needed.
	sched.refresh(context.Background())

	if client.characterDemandCalls != afterFirst {
		t.Errorf("second run made %d more character_demand calls; want 0 — the attribution should be cached",
			client.characterDemandCalls-afterFirst)
	}
	row, ok := repo.cards[strconv.Itoa(1)]
	if !ok || row.CharacterName == nil {
		t.Fatalf("card 1 lost its character attribution across runs: %+v", row)
	}
	if *row.CharacterName != "Charizard" {
		t.Errorf("card 1 character = %q; want %q", *row.CharacterName, "Charizard")
	}
	if got, _ := blobQuality(t, repo, "Charizard"); got != demand.QualityFull {
		t.Errorf("after second run data_quality = %q; want %q", got, demand.QualityFull)
	}
}

func TestDHAnalyticsRefresh_AttributionIsCappedPerRun(t *testing.T) {
	seedIDs := make([]int, 0, maxCharacterLookupsPerRun+10)
	cardToCharacter := make(map[int]string, cap(seedIDs))
	for i := 1; i <= maxCharacterLookupsPerRun+10; i++ {
		seedIDs = append(seedIDs, i)
		cardToCharacter[i] = "Charizard"
	}
	client := analyticsClientFor(cardToCharacter, map[int]string{})
	repo := newAttributionRepo()
	sched := NewDHAnalyticsRefreshScheduler(
		client, repo, &fakeCardLister{ids: seedIDs}, mocks.NewMockLogger(), newTestCfg())

	sched.refresh(context.Background())

	if client.characterDemandCalls > maxCharacterLookupsPerRun {
		t.Errorf("made %d character_demand calls; want at most %d",
			client.characterDemandCalls, maxCharacterLookupsPerRun)
	}
}

func TestSoleCharacterName(t *testing.T) {
	tests := []struct {
		name string
		resp *dh.CharacterDemandResponse
		want string
	}{
		{name: "nil response attributes nothing", resp: nil, want: ""},
		{
			name: "empty response attributes nothing",
			resp: &dh.CharacterDemandResponse{},
			want: "",
		},
		{
			name: "the single entry wins",
			resp: &dh.CharacterDemandResponse{CharacterDemand: []dh.CharacterDemandEntry{
				{CharacterName: "Charizard", CardCount: 1},
			}},
			want: "Charizard",
		},
		{
			name: "blank names are skipped",
			resp: &dh.CharacterDemandResponse{CharacterDemand: []dh.CharacterDemandEntry{
				{CharacterName: "", CardCount: 9},
				{CharacterName: "Mewtwo", CardCount: 1},
			}},
			want: "Mewtwo",
		},
		{
			name: "the largest card count wins when DH returns several",
			resp: &dh.CharacterDemandResponse{CharacterDemand: []dh.CharacterDemandEntry{
				{CharacterName: "Mewtwo", CardCount: 1},
				{CharacterName: "Charizard", CardCount: 4},
			}},
			want: "Charizard",
		},
		{
			name: "ties break on name so repeated runs agree",
			resp: &dh.CharacterDemandResponse{CharacterDemand: []dh.CharacterDemandEntry{
				{CharacterName: "Mewtwo", CardCount: 2},
				{CharacterName: "Charizard", CardCount: 2},
			}},
			want: "Charizard",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := soleCharacterName(tt.resp); got != tt.want {
				t.Errorf("soleCharacterName() = %q; want %q", got, tt.want)
			}
		})
	}
}
