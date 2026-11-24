package mocks

import "net/http"

// HTTPTransportFunc captures outbound requests without external network access.
type HTTPTransportFunc func(*http.Request) (*http.Response, error)

func (f HTTPTransportFunc) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }
