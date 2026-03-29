package cardladder

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
)

const defaultFirestoreBaseURL = "https://firestore.googleapis.com/v1"
const firestoreProject = "cardladder-71d53"

// FirestoreCardData holds the gemRate fields from a Firestore collection card document.
type FirestoreCardData struct {
	GemRateID        string
	GemRateCondition string
}

// firestoreListResponse is the Firestore REST list documents response.
type firestoreListResponse struct {
	Documents     []firestoreDocument `json:"documents"`
	NextPageToken string              `json:"nextPageToken"`
}

type firestoreDocument struct {
	Name   string                    `json:"name"`
	Fields map[string]firestoreValue `json:"fields"`
}

type firestoreValue struct {
	StringValue  *string `json:"stringValue,omitempty"`
	IntegerValue *string `json:"integerValue,omitempty"`
	BooleanValue *bool   `json:"booleanValue,omitempty"`
}

// FetchFirestoreCards lists all collection card documents from Firestore
// and returns a map of collectionCardId → FirestoreCardData.
func (c *Client) FetchFirestoreCards(ctx context.Context, uid, collectionID string) (map[string]FirestoreCardData, error) {
	result := make(map[string]FirestoreCardData)
	pageToken := ""

	basePath := fmt.Sprintf("projects/%s/databases/(default)/documents/users/%s/collections/%s/collection_cards",
		firestoreProject, uid, collectionID)

	for {
		if err := c.rateLimiter.Wait(ctx); err != nil {
			return nil, err
		}

		token, err := c.getToken(ctx)
		if err != nil {
			return nil, fmt.Errorf("get auth token: %w", err)
		}

		u, _ := url.Parse(defaultFirestoreBaseURL + "/" + basePath)
		q := u.Query()
		q.Set("pageSize", "100")
		if pageToken != "" {
			q.Set("pageToken", pageToken)
		}
		u.RawQuery = q.Encode()

		req, err := http.NewRequestWithContext(ctx, http.MethodGet, u.String(), nil)
		if err != nil {
			return nil, fmt.Errorf("create firestore request: %w", err)
		}
		req.Header.Set("Authorization", "Bearer "+token)

		resp, err := c.httpClient.Do(req)
		if err != nil {
			return nil, fmt.Errorf("firestore request: %w", err)
		}

		body, err := io.ReadAll(resp.Body)
		resp.Body.Close() //nolint:errcheck
		if err != nil {
			return nil, fmt.Errorf("read firestore response: %w", err)
		}
		if resp.StatusCode != http.StatusOK {
			return nil, fmt.Errorf("firestore returned status %d: %s", resp.StatusCode, body)
		}

		var listResp firestoreListResponse
		if err := json.Unmarshal(body, &listResp); err != nil {
			return nil, fmt.Errorf("unmarshal firestore response: %w", err)
		}

		for _, doc := range listResp.Documents {
			cardID := firestoreString(doc.Fields, "collectionCardId")
			if cardID == "" {
				continue
			}
			result[cardID] = FirestoreCardData{
				GemRateID:        firestoreString(doc.Fields, "gemRateId"),
				GemRateCondition: firestoreString(doc.Fields, "gemRateCondition"),
			}
		}

		if listResp.NextPageToken == "" {
			break
		}
		pageToken = listResp.NextPageToken
	}

	return result, nil
}

func firestoreString(fields map[string]firestoreValue, key string) string {
	v, ok := fields[key]
	if !ok || v.StringValue == nil {
		return ""
	}
	return *v.StringValue
}
