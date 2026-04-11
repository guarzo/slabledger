package cardutil

import (
	"context"

	"github.com/guarzo/slabledger/internal/domain/observability"
)

type traceKeyType struct{}

var traceKey traceKeyType

// NormalizationTrace records the input/output of each normalization step.
// When nil (the default), all methods are no-ops.
type NormalizationTrace struct {
	Steps []NormalizationStep
}

// NormalizationStep captures a single transformation in the normalization chain.
type NormalizationStep struct {
	Function string
	Input    string
	Output   string
}

// AddStep records a normalization step. Safe to call on nil receiver (no-op).
func (t *NormalizationTrace) AddStep(fn, input, output string) {
	if t == nil {
		return
	}
	t.Steps = append(t.Steps, NormalizationStep{Function: fn, Input: input, Output: output})
}

// ContextWithTrace returns a new context carrying a fresh NormalizationTrace.
func ContextWithTrace(ctx context.Context) context.Context {
	return context.WithValue(ctx, traceKey, &NormalizationTrace{})
}

// TraceFromContext returns the NormalizationTrace from ctx, or nil if none.
func TraceFromContext(ctx context.Context) *NormalizationTrace {
	t, ok := ctx.Value(traceKey).(*NormalizationTrace)
	if !ok {
		return nil
	}
	return t
}

// LogNormalizationTrace emits each step of the normalization trace at Debug level.
// No-op if trace is empty or logger is nil.
func LogNormalizationTrace(ctx context.Context, logger observability.Logger, card, set string) {
	trace := TraceFromContext(ctx)
	if trace == nil || len(trace.Steps) == 0 || logger == nil {
		return
	}
	for i, step := range trace.Steps {
		logger.Debug(ctx, "normalization trace",
			observability.String("card", card),
			observability.String("set", set),
			observability.Int("step", i),
			observability.String("fn", step.Function),
			observability.String("input", step.Input),
			observability.String("output", step.Output))
	}
}
