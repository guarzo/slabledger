package cardutil

import (
	"context"
	"testing"
)

func TestNormalizationTrace(t *testing.T) {
	// Nil trace is a no-op
	var nilTrace *NormalizationTrace
	nilTrace.AddStep("fn", "in", "out") // should not panic

	// No trace on plain context
	if got := TraceFromContext(context.Background()); got != nil {
		t.Errorf("TraceFromContext on plain context: got %v, want nil", got)
	}

	// Trace round-trip through context
	ctx := ContextWithTrace(context.Background())
	trace := TraceFromContext(ctx)
	if trace == nil {
		t.Fatal("TraceFromContext returned nil after ContextWithTrace")
	}
	if len(trace.Steps) != 0 {
		t.Fatalf("fresh trace has %d steps, want 0", len(trace.Steps))
	}

	// AddStep records correctly
	trace.AddStep("NormalizePurchaseName", "MEWTWO-HOLO", "MEWTWO Holo")
	trace.AddStep("SimplifyForSearch", "MEWTWO Holo", "MEWTWO")
	if len(trace.Steps) != 2 {
		t.Fatalf("trace has %d steps, want 2", len(trace.Steps))
	}
	if trace.Steps[0].Function != "NormalizePurchaseName" {
		t.Errorf("step 0 function: got %q, want %q", trace.Steps[0].Function, "NormalizePurchaseName")
	}
	if trace.Steps[0].Input != "MEWTWO-HOLO" {
		t.Errorf("step 0 input: got %q, want %q", trace.Steps[0].Input, "MEWTWO-HOLO")
	}
	if trace.Steps[0].Output != "MEWTWO Holo" {
		t.Errorf("step 0 output: got %q, want %q", trace.Steps[0].Output, "MEWTWO Holo")
	}

	// Same trace is visible deeper in context chain
	type childKeyType struct{}
	childCtx := context.WithValue(ctx, childKeyType{}, "unrelated")
	if got := TraceFromContext(childCtx); got != trace {
		t.Error("trace not inherited through child context")
	}
}
