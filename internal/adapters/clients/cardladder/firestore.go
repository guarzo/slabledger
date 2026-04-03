package cardladder

import (
	"context"
	"encoding/json"
	"fmt"
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

		u, err := url.Parse(defaultFirestoreBaseURL + "/" + basePath)
		if err != nil {
			return nil, fmt.Errorf("parse firestore URL: %w", err)
		}
		q := u.Query()
		q.Set("pageSize", "100")
		if pageToken != "" {
			q.Set("pageToken", pageToken)
		}
		u.RawQuery = q.Encode()

		headers := map[string]string{
			"Authorization": "Bearer " + token,
		}

		resp, err := c.httpClient.Get(ctx, u.String(), headers, 0)
		if err != nil {
			return nil, fmt.Errorf("firestore request: %w", err)
		}

		var listResp firestoreListResponse
		if err := json.Unmarshal(resp.Body, &listResp); err != nil {
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
