package main

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func readinessSourceQuery() url.Values {
	return url.Values{"index": {"salesarchive"}, "sort": {"date"}, "direction": {"desc"}, "limit": {"100"}, "page": {"0"}, "filters": {"condition:g10|profileId:psa-1|gradingCompany:psa"}}
}

func requestReadinessSource(t *testing.T, server *httptest.Server, q url.Values) (int, []byte) {
	t.Helper()
	client := &http.Client{Timeout: time.Second}
	req, err := http.NewRequest(http.MethodGet, server.URL+"?"+q.Encode(), nil)
	require.NoError(t, err)
	req.Header.Set("Authorization", "Bearer source-fixture")
	response, err := client.Do(req)
	require.NoError(t, err, "source requests must receive HTTP diagnostics, not terminate the handler")
	body, err := io.ReadAll(response.Body)
	require.NoError(t, response.Body.Close())
	require.NoError(t, err)
	return response.StatusCode, body
}

func TestReadinessSourceMalformedRequest(t *testing.T) {
	for _, tc := range []struct{ key, value string }{
		{"index", "other"}, {"sort", "price"}, {"direction", "asc"}, {"limit", "1"}, {"page", "2"},
		{"filters", "broken"}, {"filters", "condition:g9|profileId:psa-1|gradingCompany:psa"},
		{"filters", "condition:g10|profileId:psa-1|gradingCompany:bgs"},
		{"filters", "condition:g10|wrong:psa-1|gradingCompany:psa"},
		{"filters", "condition:g10|profileId:|gradingCompany:psa"},
	} {
		t.Run(tc.key+"/"+tc.value, func(t *testing.T) {
			f := &readinessSourceFixture{now: time.Now().UTC(), started: time.Now()}
			failures := make(chan string, 8)
			server := httptest.NewServer(readinessHandler(func(format string, args ...any) {
				failures <- fmt.Sprintf(format, args...)
			}, f.serve))
			defer server.Close()
			q := readinessSourceQuery()
			q.Set(tc.key, tc.value)
			status, body := requestReadinessSource(t, server, q)
			require.Equal(t, http.StatusInternalServerError, status)
			require.Contains(t, string(body), tc.key)
			require.Len(t, failures, 1, "unexpected errors must be recorded even if a browser tolerates HTTP 500")
			require.Contains(t, <-failures, tc.key)
			status, body = requestReadinessSource(t, server, readinessSourceQuery())
			require.Equal(t, http.StatusOK, status)
			var response struct {
				Hits      []json.RawMessage
				TotalHits int
			}
			require.NoError(t, json.Unmarshal(body, &response))
			require.Len(t, response.Hits, 2)
			require.Equal(t, 2, response.TotalHits)
			require.Empty(t, failures)
			server.Close()
			f.mu.Lock()
			calls := append([]url.Values{}, f.calls...)
			f.mu.Unlock()
			require.Equal(t, []url.Values{readinessSourceQuery()}, calls)
		})
	}
}

func TestReadinessSourceControlledFailures(t *testing.T) {
	for _, mode := range []string{"failed", "partial"} {
		t.Run(mode, func(t *testing.T) {
			f := &readinessSourceFixture{now: time.Now().UTC(), started: time.Now(), mode: mode}
			server := startReadinessSource(t, f)
			status, body := requestReadinessSource(t, server, readinessSourceQuery())
			if mode == "failed" {
				require.Equal(t, http.StatusUnauthorized, status)
				require.Contains(t, string(body), "controlled source authorization failure")
			} else {
				require.Equal(t, http.StatusOK, status)
				var response struct {
					Hits      []json.RawMessage
					TotalHits int
				}
				require.NoError(t, json.Unmarshal(body, &response))
				require.Len(t, response.Hits, 2)
				require.Equal(t, 3, response.TotalHits, "the real adapter must still detect the incomplete page")
			}
			f.setMode("complete")
			status, _ = requestReadinessSource(t, server, readinessSourceQuery())
			require.Equal(t, http.StatusOK, status)
		})
	}
}
