package picks

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/guarzo/slabledger/internal/domain/ai"
	"github.com/guarzo/slabledger/internal/domain/observability"
)

// ---------------------------------------------------------------------------
// Test helpers — local mocks to avoid import cycles with testutil/mocks
// ---------------------------------------------------------------------------

type nopLogger struct{}

func (n *nopLogger) Debug(_ context.Context, _ string, _ ...observability.Field) {}
func (n *nopLogger) Info(_ context.Context, _ string, _ ...observability.Field)  {}
func (n *nopLogger) Warn(_ context.Context, _ string, _ ...observability.Field)  {}
func (n *nopLogger) Error(_ context.Context, _ string, _ ...observability.Field) {}
func (n *nopLogger) With(_ context.Context, _ ...observability.Field) observability.Logger {
	return n
}

type mockLLM struct {
	responses []string
	err       error
	calls     int
}

func (m *mockLLM) StreamCompletion(_ context.Context, _ ai.CompletionRequest, stream func(ai.CompletionChunk)) error {
	if m.err != nil {
		m.calls++
		return m.err
	}
	idx := m.calls
	m.calls++
	if idx < len(m.responses) {
		stream(ai.CompletionChunk{Delta: m.responses[idx]})
	}
	return nil
}

type mockRepo struct {
	SavePicksFn                 func(ctx context.Context, p []Pick) error
	GetPicksByDateFn            func(ctx context.Context, date time.Time) ([]Pick, error)
	GetPicksRangeFn             func(ctx context.Context, from, to time.Time) ([]Pick, error)
	PicksExistForDateFn         func(ctx context.Context, date time.Time) (bool, error)
	SaveWatchlistItemFn         func(ctx context.Context, item WatchlistItem) error
	DeleteWatchlistItemFn       func(ctx context.Context, id int) error
	GetActiveWatchlistFn        func(ctx context.Context) ([]WatchlistItem, error)
	UpdateWatchlistAssessmentFn func(ctx context.Context, watchlistID int, pickID int) error
}

func (m *mockRepo) SavePicks(ctx context.Context, p []Pick) error {
	if m.SavePicksFn != nil {
		return m.SavePicksFn(ctx, p)
	}
	return nil
}
func (m *mockRepo) GetPicksByDate(ctx context.Context, date time.Time) ([]Pick, error) {
	if m.GetPicksByDateFn != nil {
		return m.GetPicksByDateFn(ctx, date)
	}
	return nil, nil
}
func (m *mockRepo) GetPicksRange(ctx context.Context, from, to time.Time) ([]Pick, error) {
	if m.GetPicksRangeFn != nil {
		return m.GetPicksRangeFn(ctx, from, to)
	}
	return nil, nil
}
func (m *mockRepo) PicksExistForDate(ctx context.Context, date time.Time) (bool, error) {
	if m.PicksExistForDateFn != nil {
		return m.PicksExistForDateFn(ctx, date)
	}
	return false, nil
}
func (m *mockRepo) SaveWatchlistItem(ctx context.Context, item WatchlistItem) error {
	if m.SaveWatchlistItemFn != nil {
		return m.SaveWatchlistItemFn(ctx, item)
	}
	return nil
}
func (m *mockRepo) DeleteWatchlistItem(ctx context.Context, id int) error {
	if m.DeleteWatchlistItemFn != nil {
		return m.DeleteWatchlistItemFn(ctx, id)
	}
	return nil
}
func (m *mockRepo) GetActiveWatchlist(ctx context.Context) ([]WatchlistItem, error) {
	if m.GetActiveWatchlistFn != nil {
		return m.GetActiveWatchlistFn(ctx)
	}
	return nil, nil
}
func (m *mockRepo) UpdateWatchlistAssessment(ctx context.Context, watchlistID int, pickID int) error {
	if m.UpdateWatchlistAssessmentFn != nil {
		return m.UpdateWatchlistAssessmentFn(ctx, watchlistID, pickID)
	}
	return nil
}

type mockProfitability struct{}

func (m *mockProfitability) GetProfitablePatterns(_ context.Context) (ProfitabilityProfile, error) {
	return ProfitabilityProfile{}, nil
}

type mockInventory struct{}

func (m *mockInventory) GetHeldCardNames(_ context.Context) ([]string, error) {
	return nil, nil
}

// ---------------------------------------------------------------------------
// Test data
// ---------------------------------------------------------------------------

var validCandidatesJSON = `[{"card_name":"Charizard ex","set_name":"Obsidian Flames","grade":"PSA 10","rationale":"test"}]`

var validScoredJSON = `[{"card_name":"Charizard ex","set_name":"Obsidian Flames","grade":"PSA 10","direction":"buy","confidence":"high","buy_thesis":"Strong demand","target_buy_price_cents":15000,"expected_sell_price_cents":22500,"rank":1,"signals":[{"factor":"population","direction":"bullish","title":"Low pop","detail":"Only 847"}]}]`

// ---------------------------------------------------------------------------
// Tests
// ---------------------------------------------------------------------------

func TestGenerateDailyPicks_Idempotent(t *testing.T) {
	repo := &mockRepo{
		PicksExistForDateFn: func(_ context.Context, _ time.Time) (bool, error) {
			return true, nil
		},
	}
	llm := &mockLLM{}
	svc := NewService(repo, llm, &mockProfitability{}, &mockInventory{}, &nopLogger{})

	err := svc.GenerateDailyPicks(context.Background())
	if !errors.Is(err, ErrPicksAlreadyExist) {
		t.Fatalf("expected ErrPicksAlreadyExist, got: %v", err)
	}
	if llm.calls != 0 {
		t.Errorf("expected 0 LLM calls, got %d", llm.calls)
	}
}

func TestGenerateDailyPicks_Success(t *testing.T) {
	var savedPicks []Pick
	repo := &mockRepo{
		SavePicksFn: func(_ context.Context, p []Pick) error {
			savedPicks = p
			return nil
		},
	}
	llm := &mockLLM{
		responses: []string{validCandidatesJSON, validScoredJSON},
	}
	svc := NewService(repo, llm, &mockProfitability{}, &mockInventory{}, &nopLogger{})

	err := svc.GenerateDailyPicks(context.Background())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if llm.calls != 2 {
		t.Errorf("expected 2 LLM calls, got %d", llm.calls)
	}
	if len(savedPicks) == 0 {
		t.Fatal("expected picks to be saved, got none")
	}
	if savedPicks[0].Source != SourceAI {
		t.Errorf("expected source %q, got %q", SourceAI, savedPicks[0].Source)
	}
}

func TestGenerateDailyPicks_LLMFailure(t *testing.T) {
	saveCalled := false
	repo := &mockRepo{
		SavePicksFn: func(_ context.Context, _ []Pick) error {
			saveCalled = true
			return nil
		},
	}
	llm := &mockLLM{err: errors.New("azure unavailable")}
	svc := NewService(repo, llm, &mockProfitability{}, &mockInventory{}, &nopLogger{})

	err := svc.GenerateDailyPicks(context.Background())
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if !errors.Is(err, ErrLLMFailure) {
		t.Errorf("expected error to wrap ErrLLMFailure, got: %v", err)
	}
	if saveCalled {
		t.Error("expected SavePicks not to be called on LLM failure")
	}
}

func TestGetLatestPicks(t *testing.T) {
	want := []Pick{{CardName: "Pikachu", SetName: "Base Set", Grade: "PSA 9"}}
	repo := &mockRepo{
		GetPicksByDateFn: func(_ context.Context, _ time.Time) ([]Pick, error) {
			return want, nil
		},
	}
	svc := NewService(repo, &mockLLM{}, &mockProfitability{}, &mockInventory{}, &nopLogger{})

	got, err := svc.GetLatestPicks(context.Background())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(got) != len(want) {
		t.Fatalf("expected %d picks, got %d", len(want), len(got))
	}
	if got[0].CardName != want[0].CardName {
		t.Errorf("card name = %q, want %q", got[0].CardName, want[0].CardName)
	}
}

func TestWatchlistCRUD(t *testing.T) {
	t.Run("AddToWatchlist sets manual source", func(t *testing.T) {
		var saved WatchlistItem
		repo := &mockRepo{
			SaveWatchlistItemFn: func(_ context.Context, item WatchlistItem) error {
				saved = item
				return nil
			},
		}
		svc := NewService(repo, &mockLLM{}, &mockProfitability{}, &mockInventory{}, &nopLogger{})

		item := WatchlistItem{CardName: "Charizard", SetName: "Base Set", Grade: "PSA 10"}
		if err := svc.AddToWatchlist(context.Background(), item); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if saved.Source != WatchlistManual {
			t.Errorf("source = %q, want %q", saved.Source, WatchlistManual)
		}
	})

	t.Run("RemoveFromWatchlist passes id", func(t *testing.T) {
		var deletedID int
		repo := &mockRepo{
			DeleteWatchlistItemFn: func(_ context.Context, id int) error {
				deletedID = id
				return nil
			},
		}
		svc := NewService(repo, &mockLLM{}, &mockProfitability{}, &mockInventory{}, &nopLogger{})

		if err := svc.RemoveFromWatchlist(context.Background(), 42); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if deletedID != 42 {
			t.Errorf("deleted id = %d, want 42", deletedID)
		}
	})

	t.Run("GetWatchlist returns items", func(t *testing.T) {
		want := []WatchlistItem{{CardName: "Lugia", SetName: "Neo Genesis", Grade: "PSA 10", Active: true}}
		repo := &mockRepo{
			GetActiveWatchlistFn: func(_ context.Context) ([]WatchlistItem, error) {
				return want, nil
			},
		}
		svc := NewService(repo, &mockLLM{}, &mockProfitability{}, &mockInventory{}, &nopLogger{})

		got, err := svc.GetWatchlist(context.Background())
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if len(got) != 1 || got[0].CardName != "Lugia" {
			t.Errorf("got %+v, want %+v", got, want)
		}
	})
}

func TestParseCandidates(t *testing.T) {
	tests := []struct {
		name    string
		input   string
		wantLen int
		wantErr bool
	}{
		{name: "valid JSON", input: validCandidatesJSON, wantLen: 1},
		{name: "markdown-fenced JSON", input: "```json\n" + validCandidatesJSON + "\n```", wantLen: 1},
		{name: "plain fenced JSON", input: "```\n" + validCandidatesJSON + "\n```", wantLen: 1},
		{name: "invalid JSON", input: "not json at all", wantErr: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := ParseCandidates(tt.input)
			if tt.wantErr {
				if err == nil {
					t.Fatal("expected error, got nil")
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if len(got) != tt.wantLen {
				t.Errorf("len = %d, want %d", len(got), tt.wantLen)
			}
		})
	}
}

func TestParseScoredPicks(t *testing.T) {
	picks, err := ParseScoredPicks(validScoredJSON)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(picks) != 1 {
		t.Fatalf("expected 1 pick, got %d", len(picks))
	}

	p := picks[0]
	if p.CardName != "Charizard ex" {
		t.Errorf("CardName = %q, want %q", p.CardName, "Charizard ex")
	}
	if p.Direction != DirectionBuy {
		t.Errorf("Direction = %q, want %q", p.Direction, DirectionBuy)
	}
	if p.Confidence != ConfidenceHigh {
		t.Errorf("Confidence = %q, want %q", p.Confidence, ConfidenceHigh)
	}
	if p.TargetBuyPrice != 15000 {
		t.Errorf("TargetBuyPrice = %d, want 15000", p.TargetBuyPrice)
	}
	if p.ExpectedSellPrice != 22500 {
		t.Errorf("ExpectedSellPrice = %d, want 22500", p.ExpectedSellPrice)
	}
	if len(p.Signals) != 1 {
		t.Fatalf("expected 1 signal, got %d", len(p.Signals))
	}
	if p.Signals[0].Factor != "population" {
		t.Errorf("Signal.Factor = %q, want %q", p.Signals[0].Factor, "population")
	}
	if p.Signals[0].Direction != SignalBullish {
		t.Errorf("Signal.Direction = %q, want %q", p.Signals[0].Direction, SignalBullish)
	}
}
