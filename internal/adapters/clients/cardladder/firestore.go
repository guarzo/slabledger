package cardladder

import (
	"context"
	"encoding/json"
	"fmt"
	"net/url"
	"strconv"
	"time"

	"github.com/guarzo/slabledger/internal/adapters/clients/httpx"
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
	StringValue    *string  `json:"stringValue,omitempty"`
	IntegerValue   *string  `json:"integerValue,omitempty"`
	BooleanValue   *bool    `json:"booleanValue,omitempty"`
	TimestampValue *string  `json:"timestampValue,omitempty"`
	DoubleValue    *float64 `json:"doubleValue,omitempty"`
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

// CreateCollectionCard creates a new card document in a CardLadder Firestore collection.
// It uses the Firestore REST API to write the document with auto-generated ID.
func (c *Client) CreateCollectionCard(ctx context.Context, uid, collectionID string, input AddCollectionCardInput) (string, error) {
	if err := c.rateLimiter.Wait(ctx); err != nil {
		return "", err
	}

	token, err := c.getToken(ctx)
	if err != nil {
		return "", fmt.Errorf("get auth token: %w", err)
	}

	basePath := fmt.Sprintf("projects/%s/databases/(default)/documents/users/%s/collections/%s/collection_cards",
		firestoreProject, uid, collectionID)
	u := defaultFirestoreBaseURL + "/" + basePath

	doc := buildCardDocument(uid, collectionID, input)
	body, err := json.Marshal(doc)
	if err != nil {
		return "", fmt.Errorf("marshal firestore document: %w", err)
	}

	headers := map[string]string{
		"Authorization": "Bearer " + token,
		"Content-Type":  "application/json",
	}

	resp, err := c.httpClient.Post(ctx, u, headers, body, 0)
	if err != nil {
		return "", fmt.Errorf("firestore create: %w", err)
	}

	var created firestoreDocument
	if err := json.Unmarshal(resp.Body, &created); err != nil {
		return "", fmt.Errorf("unmarshal create response: %w", err)
	}

	return created.Name, nil
}

// DeleteCollectionCard deletes a card document from a CardLadder Firestore collection.
// The documentName must be the full resource path returned by CreateCollectionCard
// (e.g. "projects/cardladder-71d53/databases/(default)/documents/users/{uid}/collections/{cid}/collection_cards/{docId}").
func (c *Client) DeleteCollectionCard(ctx context.Context, documentName string) error {
	if err := c.rateLimiter.Wait(ctx); err != nil {
		return err
	}

	token, err := c.getToken(ctx)
	if err != nil {
		return fmt.Errorf("get auth token: %w", err)
	}

	u := defaultFirestoreBaseURL + "/" + documentName

	headers := map[string]string{
		"Authorization": "Bearer " + token,
	}

	_, err = c.httpClient.Do(ctx, httpx.Request{
		Method:  "DELETE",
		URL:     u,
		Headers: headers,
	})
	if err != nil {
		return fmt.Errorf("firestore delete %s: %w", documentName, err)
	}

	return nil
}

// buildCardDocument constructs a Firestore document matching the CardLadder
// collection card schema observed in the Fiddler captures.
func buildCardDocument(uid, collectionID string, input AddCollectionCardInput) firestoreDocument {
	now := time.Now().UTC().Format(time.RFC3339Nano)
	profit := input.CurrentValue - input.Investment

	// Construct PSA cert photo URLs if not provided
	imageURL := input.ImageURL
	imageCustom := ""
	if imageURL == "" && input.SlabSerial != "" {
		imageURL = psaCertThumbnailURL(input.SlabSerial)
		imageCustom = psaCertImageURL(input.SlabSerial)
	}

	fields := map[string]firestoreValue{
		"uid":              fsString(uid),
		"collectionId":     fsString(collectionID),
		"label":            fsString(input.Label),
		"player":           fsString(input.Player),
		"playerIndexId":    fsString(input.PlayerIndexID),
		"category":         fsString(input.Category),
		"year":             fsString(input.Year),
		"set":              fsString(input.Set),
		"number":           fsString(input.Number),
		"variation":        fsString(input.Variation),
		"condition":        fsString(input.Condition),
		"gradingCompany":   fsString(input.GradingCompany),
		"gemRateId":        fsString(input.GemRateID),
		"gemRateCondition": fsString(input.GemRateCondition),
		"slabSerial":       fsString(input.SlabSerial),
		"image":            fsString(imageURL),
		"imageBack":        fsString(input.ImageBackURL),
		"imageCustom":      fsString(imageCustom),
		"pop":              fsInt(input.Pop),
		"currentValue":     fsInt(int(input.CurrentValue)),
		"investment":       fsInt(int(input.Investment)),
		"profit":           fsInt(int(profit)),
		"quantity":         fsInt(1),
		"quantitySold":     fsInt(0),
		"sold":             fsBool(false),
		"hidden":           fsBool(false),
		"keyCard":          fsBool(false),
		"public":           fsBool(false),
		"ownership":        fsInt(100),
		"dateAdded":        fsTimestamp(now),
	}

	if !input.DatePurchased.IsZero() {
		fields["datePurchased"] = fsTimestamp(input.DatePurchased.UTC().Format(time.RFC3339Nano))
	}

	return firestoreDocument{Fields: fields}
}

func firestoreString(fields map[string]firestoreValue, key string) string {
	v, ok := fields[key]
	if !ok || v.StringValue == nil {
		return ""
	}
	return *v.StringValue
}

func fsString(s string) firestoreValue {
	return firestoreValue{StringValue: &s}
}

func fsInt(n int) firestoreValue {
	s := strconv.Itoa(n)
	return firestoreValue{IntegerValue: &s}
}

func fsBool(b bool) firestoreValue {
	return firestoreValue{BooleanValue: &b}
}

func fsTimestamp(ts string) firestoreValue {
	return firestoreValue{TimestampValue: &ts}
}

const psaCertCDN = "https://d1htnxwo4o0jhw.cloudfront.net/cert"

func psaCertImageURL(cert string) string {
	return fmt.Sprintf("%s/%s/%s.jpg", psaCertCDN, cert, cert)
}

func psaCertThumbnailURL(cert string) string {
	return fmt.Sprintf("%s/%s/small_%s.jpg", psaCertCDN, cert, cert)
}
