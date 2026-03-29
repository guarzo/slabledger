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
// nopLogger satisfies observability.Logger and discards all output.
// ---------------------------------------------------------------------------

type nopLogger struct{}

func (n *nopLogger) Debug(_ context.Context, _ string, _ ...observability.Field) {}
func (n *nopLogger) Info(_ context.Context, _ string, _ ...observability.Field)  {}
func (n *nopLogger) Warn(_ context.Context, _ string, _ ...observability.Field)  {}
func (n *nopLogger) Error(_ context.Context, _ string, _ ...observability.Field) {}
func (n *nopLogger) With(_ context.Context, _ ...observability.Field) observability.Logger {
	return n
}

// ---------------------------------------------------------------------------
// mockLLM implements ai.LLMProvider.
// It returns responses in-order, one per StreamCompletion call.
// If err is set, every call returns that error immediately.
// ---------------------------------------------------------------------------

type mockLLM struct {
	responses []string
	err       error
	calls     int
}

func (m *mockLLM) StreamCompletion(_ context.Context, _ ai.CompletionRequest, stream func(ai.CompletionChunk)) error {
	m.calls++
	if m.err != nil {
		return m.err
	}
	idx := m.calls - 1
	if idx < len(m.responses) {
		stream(ai.CompletionChunk{Delta: m.responses[idx]})
	}
	return nil
}

// ---------------------------------------------------------------------------
// mockRepo is an inline mock for picks.Repository.
// ---------------------------------------------------------------------------

type mockRepo struct {
	picksExistForDateFn        func(ctx context.Context, date time.Time) (bool, error)
	savePicksFn                func(ctx context.Context, p []Pick) error
	getPicksByDateFn           func(ctx context.Context, date time.Time) ([]Pick, error)
	getPicksRangeFn            func(ctx context.Context, from, to time.Time) ([]Pick, error)
	saveWatchlistItemFn        func(ctx context.Context, item WatchlistItem) error
	deleteWatchlistItemFn      func(ctx context.Context, id int) error
	getActiveWatchlistFn       func(ctx context.Context) ([]WatchlistItem, error)
	updateWatchlistAssessmentFn func(ctx context.Context, watchlistID int, pickID int) error
}

func (m *mockRepo) PicksExistForDate(ctx context.Context, date time.Time) (bool, error) {
	if m.picksExistForDateFn != nil {
		return m.picksExistForDateFn(ctx, date)
	}
	return false, nil
}

func (m *mockRepo) SavePicks(ctx context.Context, p []Pick) error {
	if m.savePicksFn != nil {
		return m.savePicksFn(ctx, p)
	}
	return nil
}

func (m *mockRepo) GetPicksByDate(ctx context.Context, date time.Time) ([]Pick, error) {
	if m.getPicksByDateFn != nil {
		return m.getPicksByDateFn(ctx, date)
	}
	return nil, nil
}

func (m *mockRepo) GetPicksRange(ctx context.Context, from, to time.Time) ([]Pick, error) {
	if m.getPicksRangeFn != nil {
		return m.getPicksRangeFn(ctx, from, to)
	}
	return nil, nil
}

func (m *mockRepo) SaveWatchlistItem(ctx context.Context, item WatchlistItem) error {
	if m.saveWatchlistItemFn != nil {
		return m.saveWatchlistItemFn(ctx, item)
	}
	return nil
}

func (m *mockRepo) DeleteWatchlistItem(ctx context.Context, id int) error {
	if m.deleteWatchlistItemFn != nil {
		return m.deleteWatchlistItemFn(ctx, id)
	}
	return nil
}

func (m *mockRepo) GetActiveWatchlist(ctx context.Context) ([]WatchlistItem, error) {
	if m.getActiveWatchlistFn != nil {
		return m.getActiveWatchlistFn(ctx)
	}
	return nil, nil
}

func (m *mockRepo) UpdateWatchlistAssessment(ctx context.Context, watchlistID int, pickID int) error {
	if m.updateWatchlistAssessmentFn != nil {
		return m.updateWatchlistAssessmentFn(ctx, watchlistID, pickID)
	}
	return nil
}

// ---------------------------------------------------------------------------
// mockProfitability is an inline mock for ProfitabilityProvider.
// ---------------------------------------------------------------------------

type mockProfitability struct {
	fn func(ctx context.Context) (ProfitabilityProfile, error)
}

func (m *mockProfitability) GetProfitablePatterns(ctx context.Context) (ProfitabilityProfile, error) {
	if m.fn != nil {
		return m.fn(ctx)
	}
	return ProfitabilityProfile{}, nil
}

// ---------------------------------------------------------------------------
// mockInventory is an inline mock for InventoryProvider.
// ---------------------------------------------------------------------------

type mockInventory struct {
	fn func(ctx context.Context) ([]string, error)
}

func (m *mockInventory) GetHeldCardNames(ctx context.Context) ([]string, error) {
	if m.fn != nil {
		return m.fn(ctx)
	}
	return nil, nil
}

// ---------------------------------------------------------------------------
// Test data
// ---------------------------------------------------------------------------

const validCandidatesJSON = `[{"card_name":"Charizard ex","set_name":"Obsidian Flames","grade":"PSA 10","rationale":"test"}]`

const validScoredJSON = `[{"card_name":"Charizard ex","set_name":"Obsidian Flames","grade":"PSA 10","direction":"buy","confidence":"high","buy_thesis":"Strong demand","target_buy_price_cents":15000,"expected_sell_price_cents":22500,"rank":1,"signals":[{"factor":"population","direction":"bullish","title":"Low pop","detail":"Only 847"}]}]`

// ---------------------------------------------------------------------------
// TestGenerateDailyPicks_Idempotent
// ---------------------------------------------------------------------------

func TestGenerateDailyPicks_Idempotent(t *testing.T) {
	repo := &mockRepo{
		picksExistForDateFn: func(_ context.Context, _ time.Time) (bool, error) {
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

// ---------------------------------------------------------------------------
// TestGenerateDailyPicks_Success
// ---------------------------------------------------------------------------

func TestGenerateDailyPicks_Success(t *testing.T) {
	var savedPicks []Pick
	repo := &mockRepo{
		savePicksFn: func(_ context.Context, p []Pick) error {
			savedPicks = p
			return nil
		},
		// GetPicksByDate is called by updateWatchlistAssessments; return empty slice.
	}
	llm := &mockLLM{
		responses: []string{
			validCandidatesJSON, // call 1: candidate generation
			validScoredJSON,     // call 2: scoring
		},
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

// ---------------------------------------------------------------------------
// TestGenerateDailyPicks_LLMFailure
// ---------------------------------------------------------------------------

func TestGenerateDailyPicks_LLMFailure(t *testing.T) {
	saveCalled := false
	repo := &mockRepo{
		savePicksFn: func(_ context.Context, _ []Pick) error {
			saveCalled = true
			return nil
		},
	}
	wantErr := errors.New("azure unavailable")
	llm := &mockLLM{err: wantErr}

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

// ---------------------------------------------------------------------------
// TestGetLatestPicks
// ---------------------------------------------------------------------------

func TestGetLatestPicks(t *testing.T) {
	want := []Pick{{CardName: "Pikachu", SetName: "Base Set", Grade: "PSA 9"}}
	repo := &mockRepo{
		getPicksByDateFn: func(_ context.Context, _ time.Time) ([]Pick, error) {
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

// ---------------------------------------------------------------------------
// TestWatchlistCRUD
// ---------------------------------------------------------------------------

func TestWatchlistCRUD(t *testing.T) {
	t.Run("AddToWatchlist saves item with correct source", func(t *testing.T) {
		var saved WatchlistItem
		repo := &mockRepo{
			saveWatchlistItemFn: func(_ context.Context, item WatchlistItem) error {
				saved = item
				return nil
			},
		}
		svc := NewService(repo, &mockLLM{}, &mockProfitability{}, &mockInventory{}, &nopLogger{})

		item := WatchlistItem{
			CardName: "Charizard",
			SetName:  "Base Set",
			Grade:    "PSA 10",
			Source:   WatchlistManual,
			Active:   true,
		}
		if err := svc.AddToWatchlist(context.Background(), item); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if saved.Source != WatchlistManual {
			t.Errorf("source = %q, want %q", saved.Source, WatchlistManual)
		}
	})

	t.Run("RemoveFromWatchlist passes correct id", func(t *testing.T) {
		var deletedID int
		repo := &mockRepo{
			deleteWatchlistItemFn: func(_ context.Context, id int) error {
				deletedID = id
				return nil
			},
		}
		svc := NewService(repo, &mockLLM{}, &mockProfitability{}, &mockInventory{}, &nopLogger{})

		const wantID = 42
		if err := svc.RemoveFromWatchlist(context.Background(), wantID); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if deletedID != wantID {
			t.Errorf("deleted id = %d, want %d", deletedID, wantID)
		}
	})

	t.Run("GetWatchlist returns items from repo", func(t *testing.T) {
		want := []WatchlistItem{{CardName: "Lugia", SetName: "Neo Genesis", Grade: "PSA 10", Active: true}}
		repo := &mockRepo{
			getActiveWatchlistFn: func(_ context.Context) ([]WatchlistItem, error) {
				return want, nil
			},
		}
		svc := NewService(repo, &mockLLM{}, &mockProfitability{}, &mockInventory{}, &nopLogger{})

		got, err := svc.GetWatchlist(context.Background())
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if len(got) != 1 || got[0].CardName != "Lugia" {
			t.Errorf("got watchlist %+v, want %+v", got, want)
		}
	})
}

// ---------------------------------------------------------------------------
// TestParseCandidates
// ---------------------------------------------------------------------------

func TestParseCandidates(t *testing.T) {
	tests := []struct {
		name    string
		input   string
		wantLen int
		wantErr bool
	}{
		{
			name:    "valid JSON",
			input:   validCandidatesJSON,
			wantLen: 1,
		},
		{
			name:    "markdown-fenced JSON",
			input:   "```json\n" + validCandidatesJSON + "\n```",
			wantLen: 1,
		},
		{
			name:    "plain fenced JSON",
			input:   "```\n" + validCandidatesJSON + "\n```",
			wantLen: 1,
		},
		{
			name:    "invalid JSON",
			input:   `not json at all`,
			wantErr: true,
		},
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

// ---------------------------------------------------------------------------
// TestParseScoredPicks
// ---------------------------------------------------------------------------

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
	if p.SetName != "Obsidian Flames" {
		t.Errorf("SetName = %q, want %q", p.SetName, "Obsidian Flames")
	}
	if p.Grade != "PSA 10" {
		t.Errorf("Grade = %q, want %q", p.Grade, "PSA 10")
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
	if p.Rank != 1 {
		t.Errorf("Rank = %d, want 1", p.Rank)
	}
	if len(p.Signals) != 1 {
		t.Fatalf("expected 1 signal, got %d", len(p.Signals))
	}
	sig := p.Signals[0]
	if sig.Factor != "population" {
		t.Errorf("Signal.Factor = %q, want %q", sig.Factor, "population")
	}
	if sig.Direction != SignalBullish {
		t.Errorf("Signal.Direction = %q, want %q", sig.Direction, SignalBullish)
	}
	if sig.Title != "Low pop" {
		t.Errorf("Signal.Title = %q, want %q", sig.Title, "Low pop")
	}
}
