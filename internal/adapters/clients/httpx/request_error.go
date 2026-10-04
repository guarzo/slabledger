package httpx

const (
	PhasePrepare      = "prepare"
	PhaseDispatch     = "dispatch"
	PhaseResponseRead = "response_read"
	PhaseResponse     = "response"
)

// RequestError preserves dispatch evidence across retries for mutation journals.
// Uncertain is true after any transport/read failure or 5xx, even if a later
// attempt was rejected. False is NOT proof of nonexecution: a received response
// still needs an endpoint-specific, source-verified rejection contract.
// A successful response likewise needs endpoint-specific receipt validation.
type RequestError struct {
	Phase     string
	Attempts  int
	Uncertain bool
	Err       error
}

func (e *RequestError) Error() string { return e.Err.Error() }
func (e *RequestError) Unwrap() error { return e.Err }
