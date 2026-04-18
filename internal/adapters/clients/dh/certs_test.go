package dh

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestClient_ResolveCert(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		require.Equal(t, http.MethodPost, r.Method)
		require.Equal(t, "/api/v1/enterprise/certs/resolve", r.URL.Path)
		require.Equal(t, "Bearer test_api_key", r.Header.Get(enterpriseAuthHeader))

		var req struct {
			Cert CertResolveRequest `json:"cert"`
		}
		require.NoError(t, json.NewDecoder(r.Body).Decode(&req))
		require.Equal(t, "12345678", req.Cert.CertNumber)
		require.Equal(t, "Charizard", req.Cert.CardName)

		resp := CertResolution{
			CertNumber:              "12345678",
			Status:                  "matched",
			DHCardID:                42,
			CardName:                "Charizard",
			SetName:                 "Base Set",
			CardNumber:              "4/102",
			Grade:                   "10.0",
			ImageURL:                "https://example.com/charizard.png",
			CurrentMarketPriceCents: 1487500,
		}
		w.Header().Set("Content-Type", "application/json")
		require.NoError(t, json.NewEncoder(w).Encode(resp))
	}))
	defer server.Close()

	c := newTestClient(server.URL)
	req := CertResolveRequest{
		CertNumber: "12345678",
		CardName:   "Charizard",
	}
	resp, err := c.ResolveCert(context.Background(), req)
	require.NoError(t, err)
	require.NotNil(t, resp)
	require.Equal(t, "12345678", resp.CertNumber)
	require.Equal(t, "matched", resp.Status)
	require.Equal(t, 42, resp.DHCardID)
	require.Equal(t, "Charizard", resp.CardName)
	require.Equal(t, "Base Set", resp.SetName)
	require.Equal(t, "10.0", resp.Grade)
	require.Equal(t, 1487500, resp.CurrentMarketPriceCents)
}

func TestClient_ResolveCertsBatch(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		require.Equal(t, http.MethodPost, r.Method)
		require.Equal(t, "/api/v1/enterprise/certs/resolve_batch", r.URL.Path)
		require.Equal(t, "Bearer test_api_key", r.Header.Get(enterpriseAuthHeader))

		var req CertResolveBatchRequest
		require.NoError(t, json.NewDecoder(r.Body).Decode(&req))
		require.Len(t, req.Certs, 2)
		require.Equal(t, "12345678", req.Certs[0].CertNumber)
		require.Equal(t, "87654321", req.Certs[1].CertNumber)

		resp := CertResolveBatchResponse{
			JobID:      "job_xyz789",
			Status:     "queued",
			TotalCerts: 2,
		}
		w.Header().Set("Content-Type", "application/json")
		require.NoError(t, json.NewEncoder(w).Encode(resp))
	}))
	defer server.Close()

	c := newTestClient(server.URL)
	certs := []CertResolveRequest{
		{CertNumber: "12345678", CardName: "Charizard"},
		{CertNumber: "87654321", CardName: "Pikachu"},
	}
	resp, err := c.ResolveCertsBatch(context.Background(), certs)
	require.NoError(t, err)
	require.Equal(t, "job_xyz789", resp.JobID)
	require.Equal(t, "queued", resp.Status)
	require.Equal(t, 2, resp.TotalCerts)
}

func TestClient_GetCertResolutionJob(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		require.Equal(t, http.MethodGet, r.Method)
		require.Equal(t, "/api/v1/enterprise/certs/resolve_batch/job_abc123", r.URL.Path)
		require.Equal(t, "Bearer test_api_key", r.Header.Get(enterpriseAuthHeader))

		resp := CertResolutionJobStatus{
			JobID:         "job_abc123",
			Status:        "completed",
			TotalCerts:    2,
			ResolvedCount: 2,
			Results: []CertResolution{
				{
					CertNumber: "12345678",
					Status:     "matched",
					DHCardID:   42,
					CardName:   "Charizard",
					SetName:    "Base Set",
					Grade:      "10.0",
				},
				{
					CertNumber: "87654321",
					Status:     "matched",
					DHCardID:   101,
					CardName:   "Pikachu",
					SetName:    "Jungle",
					Grade:      "9.0",
				},
			},
		}
		w.Header().Set("Content-Type", "application/json")
		require.NoError(t, json.NewEncoder(w).Encode(resp))
	}))
	defer server.Close()

	c := newTestClient(server.URL)
	resp, err := c.GetCertResolutionJob(context.Background(), "job_abc123")
	require.NoError(t, err)
	require.NotNil(t, resp)
	require.Equal(t, "job_abc123", resp.JobID)
	require.Equal(t, "completed", resp.Status)
	require.Equal(t, 2, resp.TotalCerts)
	require.Equal(t, 2, resp.ResolvedCount)
	require.Len(t, resp.Results, 2)
	require.Equal(t, "12345678", resp.Results[0].CertNumber)
	require.Equal(t, "matched", resp.Results[0].Status)
	require.Equal(t, 42, resp.Results[0].DHCardID)
	require.Equal(t, "87654321", resp.Results[1].CertNumber)
	require.Equal(t, "9.0", resp.Results[1].Grade)
}

func TestClient_ConfirmMatch(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		require.Equal(t, http.MethodPost, r.Method)
		require.Equal(t, "/api/v1/enterprise/certs/confirm_match", r.URL.Path)
		require.Equal(t, "Bearer test_api_key", r.Header.Get(enterpriseAuthHeader))

		var req ConfirmMatchRequest
		require.NoError(t, json.NewDecoder(r.Body).Decode(&req))
		require.Equal(t, "12345678", req.CertNumber)
		require.Equal(t, 9042, req.DHCardID)
		require.Equal(t, "Base Set", req.SetName)
		require.Equal(t, "Charizard", req.CardName)

		resp := ConfirmMatchResponse{
			CertNumber:      "12345678",
			Status:          "confirmed",
			DHCardID:        9042,
			CardName:        "Charizard",
			SetName:         "Base Set",
			CardNumber:      "4",
			MappingsCreated: []string{"psa_card_mapping"},
			AliasesLearned:  []string{"Base Set -> Base Set (Shadowless)"},
		}
		w.Header().Set("Content-Type", "application/json")
		require.NoError(t, json.NewEncoder(w).Encode(resp))
	}))
	defer server.Close()

	c := newTestClient(server.URL)
	req := ConfirmMatchRequest{
		CertNumber: "12345678",
		DHCardID:   9042,
		SetName:    "Base Set",
		CardName:   "Charizard",
	}
	resp, err := c.ConfirmMatch(context.Background(), req)
	require.NoError(t, err)
	require.NotNil(t, resp)
	require.Equal(t, "12345678", resp.CertNumber)
	require.Equal(t, "confirmed", resp.Status)
	require.Equal(t, 9042, resp.DHCardID)
	require.Equal(t, "Charizard", resp.CardName)
	require.Equal(t, "Base Set", resp.SetName)
	require.Equal(t, "4", resp.CardNumber)
	require.Equal(t, []string{"psa_card_mapping"}, resp.MappingsCreated)
	require.Equal(t, []string{"Base Set -> Base Set (Shadowless)"}, resp.AliasesLearned)
}

func TestClient_ConfirmMatchBatch(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		require.Equal(t, http.MethodPost, r.Method)
		require.Equal(t, "/api/v1/enterprise/certs/confirm_match", r.URL.Path)
		require.Equal(t, "Bearer test_api_key", r.Header.Get(enterpriseAuthHeader))

		var req ConfirmMatchBatchRequest
		require.NoError(t, json.NewDecoder(r.Body).Decode(&req))
		require.Len(t, req.Confirmations, 2)
		require.Equal(t, "12345678", req.Confirmations[0].CertNumber)
		require.Equal(t, 9042, req.Confirmations[0].DHCardID)
		require.Equal(t, "87654321", req.Confirmations[1].CertNumber)
		require.Equal(t, 1503, req.Confirmations[1].DHCardID)

		resp := ConfirmMatchBatchResponse{
			Confirmed: 2,
			Failed:    0,
			Results: []ConfirmMatchResponse{
				{
					CertNumber:      "12345678",
					Status:          "confirmed",
					DHCardID:        9042,
					CardName:        "Charizard",
					SetName:         "Base Set",
					MappingsCreated: []string{"psa_card_mapping"},
				},
				{
					CertNumber:      "87654321",
					Status:          "confirmed",
					DHCardID:        1503,
					CardName:        "Pikachu",
					SetName:         "Jungle",
					MappingsCreated: []string{"psa_card_mapping"},
				},
			},
		}
		w.Header().Set("Content-Type", "application/json")
		require.NoError(t, json.NewEncoder(w).Encode(resp))
	}))
	defer server.Close()

	c := newTestClient(server.URL)
	confirmations := []ConfirmMatchRequest{
		{CertNumber: "12345678", DHCardID: 9042, SetName: "Base Set"},
		{CertNumber: "87654321", DHCardID: 1503, SetName: "Jungle"},
	}
	resp, err := c.ConfirmMatchBatch(context.Background(), confirmations)
	require.NoError(t, err)
	require.NotNil(t, resp)
	require.Equal(t, 2, resp.Confirmed)
	require.Equal(t, 0, resp.Failed)
	require.Len(t, resp.Results, 2)
	require.Equal(t, "12345678", resp.Results[0].CertNumber)
	require.Equal(t, "confirmed", resp.Results[0].Status)
	require.Equal(t, 9042, resp.Results[0].DHCardID)
	require.Equal(t, "87654321", resp.Results[1].CertNumber)
	require.Equal(t, "confirmed", resp.Results[1].Status)
	require.Equal(t, 1503, resp.Results[1].DHCardID)
}
