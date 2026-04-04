package advisor

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/guarzo/slabledger/internal/domain/ai"
)

// mockLLMProvider is an inline mock for ai.LLMProvider.
type mockLLMProvider struct {
	// calls tracks the number of times StreamCompletion was called.
	calls int
	// responses is a list of functions, each invoked in order.
	// Each function receives the request and calls the stream callback.
	responses []func(req CompletionRequest, stream func(CompletionChunk)) error
}

func (m *mockLLMProvider) StreamCompletion(ctx context.Context, req CompletionRequest, stream func(CompletionChunk)) error {
	idx := m.calls
	m.calls++
	if idx < len(m.responses) {
		return m.responses[idx](req, stream)
	}
	// Default: return empty content with no tool calls.
	stream(CompletionChunk{Delta: "default response"})
	return nil
}

// mockToolExecutor is an inline mock for ai.FilteredToolExecutor.
type mockToolExecutor struct {
	// executeFunc is called when Execute is invoked.
	executeFunc func(ctx context.Context, toolName string, arguments string) (string, error)
	// definitions are returned by Definitions and DefinitionsFor.
	definitions []ToolDefinition
	// executeCalls tracks calls made to Execute.
	executeCalls []struct {
		ToolName  string
		Arguments string
	}
}

func (m *mockToolExecutor) Execute(ctx context.Context, toolName string, arguments string) (string, error) {
	m.executeCalls = append(m.executeCalls, struct {
		ToolName  string
		Arguments string
	}{toolName, arguments})
	if m.executeFunc != nil {
		return m.executeFunc(ctx, toolName, arguments)
	}
	return `{"ok": true}`, nil
}

func (m *mockToolExecutor) Definitions() []ToolDefinition {
	return m.definitions
}

func (m *mockToolExecutor) DefinitionsFor(names []string) []ToolDefinition {
	nameSet := make(map[string]bool, len(names))
	for _, n := range names {
		nameSet[n] = true
	}
	var result []ToolDefinition
	for _, d := range m.definitions {
		if nameSet[d.Name] {
			result = append(result, d)
		}
	}
	return result
}

// chunkContent returns an LLM response function that emits a single delta chunk.
func chunkContent(content string) func(req CompletionRequest, stream func(CompletionChunk)) error {
	return func(req CompletionRequest, stream func(CompletionChunk)) error {
		stream(CompletionChunk{Delta: content})
		return nil
	}
}

// chunkToolCall returns an LLM response function that emits a single tool call.
func chunkToolCall(id, name, args string) func(req CompletionRequest, stream func(CompletionChunk)) error {
	return func(req CompletionRequest, stream func(CompletionChunk)) error {
		stream(CompletionChunk{
			ToolCalls: []ToolCall{{ID: id, Name: name, Arguments: args}},
		})
		return nil
	}
}

// chunkError returns an LLM response function that returns an error.
func chunkError(err error) func(req CompletionRequest, stream func(CompletionChunk)) error {
	return func(req CompletionRequest, stream func(CompletionChunk)) error {
		return err
	}
}

// --- Tests ---

func TestGenerateDigest_NoToolCalls(t *testing.T) {
	llm := &mockLLMProvider{
		responses: []func(CompletionRequest, func(CompletionChunk)) error{
			chunkContent("Weekly digest content here."),
		},
	}
	executor := &mockToolExecutor{}
	svc := NewService(llm, executor)

	var events []StreamEvent
	err := svc.GenerateDigest(context.Background(), func(e StreamEvent) {
		events = append(events, e)
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Must have received at least one delta event.
	hasDelta := false
	for _, e := range events {
		if e.Type == EventDelta {
			hasDelta = true
			if e.Content != "Weekly digest content here." {
				t.Errorf("delta content = %q, want %q", e.Content, "Weekly digest content here.")
			}
		}
	}
	if !hasDelta {
		t.Error("expected at least one EventDelta event")
	}
}

func TestGenerateDigest_WithToolCalls(t *testing.T) {
	// Round 1: LLM returns a tool call.
	// Round 2: LLM returns final content.
	llm := &mockLLMProvider{
		responses: []func(CompletionRequest, func(CompletionChunk)) error{
			chunkToolCall("tc-1", "list_campaigns", `{}`),
			chunkContent("Final analysis after tool."),
		},
	}
	executor := &mockToolExecutor{
		definitions: []ai.ToolDefinition{
			{Name: "list_campaigns", Description: "Lists campaigns"},
		},
		executeFunc: func(_ context.Context, toolName, arguments string) (string, error) {
			return `{"campaigns": []}`, nil
		},
	}
	svc := NewService(llm, executor)

	var events []StreamEvent
	err := svc.GenerateDigest(context.Background(), func(e StreamEvent) {
		events = append(events, e)
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if llm.calls != 2 {
		t.Errorf("expected 2 LLM calls, got %d", llm.calls)
	}
	if len(executor.executeCalls) != 1 {
		t.Errorf("expected 1 tool execution, got %d", len(executor.executeCalls))
	}
	if executor.executeCalls[0].ToolName != "list_campaigns" {
		t.Errorf("expected tool %q, got %q", "list_campaigns", executor.executeCalls[0].ToolName)
	}

	// Verify we received tool start/result events.
	var hasToolStart, hasToolResult, hasDelta bool
	for _, e := range events {
		switch e.Type {
		case EventToolStart:
			hasToolStart = true
		case EventToolResult:
			hasToolResult = true
		case EventDelta:
			hasDelta = true
		}
	}
	if !hasToolStart {
		t.Error("expected EventToolStart event")
	}
	if !hasToolResult {
		t.Error("expected EventToolResult event")
	}
	if !hasDelta {
		t.Error("expected EventDelta event")
	}
}

func TestGenerateDigest_LLMError(t *testing.T) {
	wantErr := errors.New("LLM is unavailable")
	llm := &mockLLMProvider{
		responses: []func(CompletionRequest, func(CompletionChunk)) error{
			chunkError(wantErr),
		},
	}
	executor := &mockToolExecutor{}
	svc := NewService(llm, executor)

	err := svc.GenerateDigest(context.Background(), func(StreamEvent) {})
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if !errors.Is(err, wantErr) {
		t.Errorf("error does not wrap expected cause: %v", err)
	}
}

func TestAnalyzeCampaign_FormatsPrompt(t *testing.T) {
	campaignID := "campaign-42"
	var capturedReqs []CompletionRequest
	llm := &mockLLMProvider{
		responses: []func(CompletionRequest, func(CompletionChunk)) error{
			func(req CompletionRequest, stream func(CompletionChunk)) error {
				capturedReqs = append(capturedReqs, req)
				stream(CompletionChunk{Delta: "Campaign analysis result."})
				return nil
			},
		},
	}
	executor := &mockToolExecutor{}
	svc := NewService(llm, executor)

	err := svc.AnalyzeCampaign(context.Background(), campaignID, func(StreamEvent) {})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(capturedReqs) == 0 {
		t.Fatal("no completion requests captured")
	}
	req := capturedReqs[0]
	if len(req.Messages) == 0 {
		t.Fatal("expected at least one message in request")
	}
	userMsg := req.Messages[0]
	if !strings.Contains(userMsg.Content, campaignID) {
		t.Errorf("user message %q does not contain campaign ID %q", userMsg.Content, campaignID)
	}
}

func TestAssessPurchase_FormatsPrompt(t *testing.T) {
	purchaseReq := PurchaseAssessmentRequest{
		CampaignID:   "camp-7",
		CampaignName: "Vintage Holo",
		CardName:     "Charizard",
		SetName:      "Base Set",
		Grade:        "10",
		BuyCostCents: 50000,
		CLValueCents: 70000,
		CertNumber:   "12345678",
	}
	var capturedReqs []CompletionRequest
	llm := &mockLLMProvider{
		responses: []func(CompletionRequest, func(CompletionChunk)) error{
			func(req CompletionRequest, stream func(CompletionChunk)) error {
				capturedReqs = append(capturedReqs, req)
				stream(CompletionChunk{Delta: "Purchase assessment result."})
				return nil
			},
		},
	}
	executor := &mockToolExecutor{}
	svc := NewService(llm, executor)

	err := svc.AssessPurchase(context.Background(), purchaseReq, func(StreamEvent) {})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(capturedReqs) == 0 {
		t.Fatal("no completion requests captured")
	}
	userMsg := capturedReqs[0].Messages[0].Content
	for _, want := range []string{"Charizard", "10", "500.00", "Vintage Holo", "camp-7", "Base Set", "12345678", "700.00"} {
		if !strings.Contains(userMsg, want) {
			t.Errorf("user prompt missing %q; got:\n%s", want, userMsg)
		}
	}
}

func TestCollectDigest_ReturnsContent(t *testing.T) {
	want := "Full digest content returned synchronously."
	llm := &mockLLMProvider{
		responses: []func(CompletionRequest, func(CompletionChunk)) error{
			chunkContent(want),
		},
	}
	executor := &mockToolExecutor{}
	svc := NewService(llm, executor)

	got, err := svc.CollectDigest(context.Background())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got != want {
		t.Errorf("CollectDigest = %q, want %q", got, want)
	}
}

func TestMaxToolRounds_Exceeded(t *testing.T) {
	// LLM always returns a tool call, never content.
	// With maxToolRounds=1, the loop should exhaust after 1 round.
	llm := &mockLLMProvider{
		responses: []func(CompletionRequest, func(CompletionChunk)) error{
			chunkToolCall("tc-1", "list_campaigns", `{}`),
			chunkToolCall("tc-2", "list_campaigns", `{}`),
			chunkToolCall("tc-3", "list_campaigns", `{}`),
		},
	}
	executor := &mockToolExecutor{
		definitions: []ai.ToolDefinition{
			{Name: "list_campaigns", Description: "Lists campaigns"},
		},
	}
	svc := NewService(llm, executor, WithMaxToolRounds(1))

	err := svc.GenerateDigest(context.Background(), func(StreamEvent) {})
	if err == nil {
		t.Fatal("expected error when max tool rounds exceeded, got nil")
	}
	if !IsMaxRoundsExceeded(err) {
		t.Errorf("expected IsMaxRoundsExceeded error, got: %v", err)
	}
	// With maxToolRounds=1, LLM should have been called exactly once.
	if llm.calls != 1 {
		t.Errorf("expected 1 LLM call, got %d", llm.calls)
	}
}

func TestTruncateToolResult(t *testing.T) {
	tests := []struct {
		name       string
		input      string
		maxLen     int
		wantCutLen int // if non-zero, assert noticeIdx == this value
	}{
		{
			name:   "short result unchanged",
			input:  `{"campaigns": []}`,
			maxLen: 100,
		},
		{
			name:   "exactly at limit unchanged",
			input:  strings.Repeat("a", 100),
			maxLen: 100,
		},
		{
			name:   "over limit truncated with notice",
			input:  strings.Repeat("x", 200),
			maxLen: 100,
		},
		{
			name:       "truncates at newline boundary",
			input:      strings.Repeat("x", 60) + "\n" + strings.Repeat("y", 60),
			maxLen:     100,
			wantCutLen: 60,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := truncateToolResult(tt.input, tt.maxLen)
			if len(tt.input) <= tt.maxLen {
				if got != tt.input {
					t.Errorf("expected input unchanged, got %q", got)
				}
				return
			}
			// Truncated result should contain the notice.
			if !strings.Contains(got, "[... truncated") {
				t.Errorf("truncated result missing notice: %q", got)
			}
			// The body before the notice should not exceed maxLen.
			noticeIdx := strings.Index(got, "\n\n[... truncated")
			if noticeIdx > tt.maxLen {
				t.Errorf("body before notice is %d chars, exceeds maxLen %d", noticeIdx, tt.maxLen)
			}
			if tt.wantCutLen != 0 && noticeIdx != tt.wantCutLen {
				t.Errorf("expected truncation at %d chars, got %d", tt.wantCutLen, noticeIdx)
			}
		})
	}
}

func TestLargeToolResult_Truncated(t *testing.T) {
	// Verify that the tool-calling loop truncates large tool results.
	largeResult := strings.Repeat(`{"id":"card-1","name":"Charizard"},`, 1000) // ~35K chars
	var round2Messages []Message
	llm := &mockLLMProvider{
		responses: []func(CompletionRequest, func(CompletionChunk)) error{
			chunkToolCall("tc-1", "get_global_inventory", `{}`),
			func(req CompletionRequest, stream func(CompletionChunk)) error {
				round2Messages = req.Messages
				stream(CompletionChunk{Delta: "Analysis complete."})
				return nil
			},
		},
	}
	executor := &mockToolExecutor{
		definitions: []ai.ToolDefinition{
			{Name: "get_global_inventory", Description: "Gets inventory"},
		},
		executeFunc: func(_ context.Context, _, _ string) (string, error) {
			return largeResult, nil
		},
	}
	svc := NewService(llm, executor)

	err := svc.GenerateDigest(context.Background(), func(StreamEvent) {})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Find the tool result message.
	var toolMsg *Message
	for i, msg := range round2Messages {
		if msg.Role == RoleTool {
			toolMsg = &round2Messages[i]
			break
		}
	}
	if toolMsg == nil {
		t.Fatal("expected tool result message in round 2")
	}
	if len(toolMsg.Content) > maxToolResultChars+200 { // allow some slack for truncation notice
		t.Errorf("tool result not truncated: got %d chars, expected ≤ %d", len(toolMsg.Content), maxToolResultChars+200)
	}
	if !strings.Contains(toolMsg.Content, "[... truncated") {
		t.Errorf("truncated result missing notice")
	}
}

func TestToolExecutionError_WrappedInJSON(t *testing.T) {
	// Tool returns an error; service should convert it to JSON and continue to next LLM round.
	toolErr := errors.New("tool database unavailable")
	var round2Messages []Message
	llm := &mockLLMProvider{
		responses: []func(CompletionRequest, func(CompletionChunk)) error{
			// Round 1: return a tool call.
			chunkToolCall("tc-1", "list_campaigns", `{}`),
			// Round 2: capture the messages that include the error JSON.
			func(req CompletionRequest, stream func(CompletionChunk)) error {
				round2Messages = req.Messages
				stream(CompletionChunk{Delta: "Response after tool error."})
				return nil
			},
		},
	}
	executor := &mockToolExecutor{
		definitions: []ai.ToolDefinition{
			{Name: "list_campaigns", Description: "Lists campaigns"},
		},
		executeFunc: func(_ context.Context, toolName, _ string) (string, error) {
			return "", toolErr
		},
	}
	svc := NewService(llm, executor)

	err := svc.GenerateDigest(context.Background(), func(StreamEvent) {})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// The tool result message (RoleTool) in round 2 should contain the error as JSON.
	var toolResultMsg *Message
	for i, msg := range round2Messages {
		if msg.Role == RoleTool {
			toolResultMsg = &round2Messages[i]
			break
		}
	}
	if toolResultMsg == nil {
		t.Fatal("expected a tool result message in round 2 messages")
	}
	wantFragment := `"error"`
	if !strings.Contains(toolResultMsg.Content, wantFragment) {
		t.Errorf("tool result content %q does not contain %q", toolResultMsg.Content, wantFragment)
	}
	if !strings.Contains(toolResultMsg.Content, "tool database unavailable") {
		t.Errorf("tool result content %q does not contain original error message", toolResultMsg.Content)
	}
}
