# Card Ladder Integration Guide

Technical reference for integrating with Card Ladder's API ecosystem. Card Ladder is a graded card valuation and sales tracking platform. Their infrastructure is built on Firebase (auth, Firestore) and a Cloud Run search API.

## Architecture Overview

```
┌─────────────────────────────────────────────────────────────────┐
│                        Card Ladder APIs                         │
│                                                                 │
│  Firebase Auth          Cloud Run Search        Firestore       │
│  (login + tokens)       (collection, sales)     (card details)  │
│                                                                 │
│  identitytoolkit.       search-zzvl7ri3bq       firestore.      │
│  googleapis.com         -uc.a.run.app           googleapis.com  │
└──────┬──────────────────────┬──────────────────────┬────────────┘
       │                      │                      │
       │    Bearer token      │    Bearer token      │
       ▼                      ▼                      ▼
┌─────────────────────────────────────────────────────────────────┐
│                     Your Application                            │
│                                                                 │
│  Auth Client     →    API Client     →    Firestore Client      │
│  (token mgmt)         (search/fetch)      (document reads)     │
└─────────────────────────────────────────────────────────────────┘
```

## Authentication

Card Ladder uses Firebase email/password authentication. All API calls require a Bearer token.

### Initial Login

```
POST https://identitytoolkit.googleapis.com/v1/accounts:signInWithPassword?key={FIREBASE_API_KEY}
Content-Type: application/json

{
  "email": "user@example.com",
  "password": "...",
  "returnSecureToken": true
}
```

Response:
```json
{
  "idToken": "eyJhb...",      // JWT, use as Bearer token
  "refreshToken": "AMf-v...", // Long-lived, store encrypted
  "expiresIn": "3600",        // Seconds until idToken expires
  "localId": "xzl4x..."      // Firebase UID (needed for Firestore paths)
}
```

### Token Refresh

ID tokens expire after 1 hour. Refresh tokens don't expire (unless the user changes their password or the account is disabled).

```
POST https://securetoken.googleapis.com/v1/token?key={FIREBASE_API_KEY}
Content-Type: application/x-www-form-urlencoded

grant_type=refresh_token&refresh_token={REFRESH_TOKEN}
```

Response:
```json
{
  "id_token": "eyJhb...",
  "refresh_token": "AMf-v...",   // May rotate — always store the latest
  "expires_in": "3600"
}
```

### Firebase API Key

The API key is a public project identifier found in Card Ladder's frontend JavaScript:

```js
firebase.initializeApp({
  apiKey: 'AIza...',
  authDomain: 'cardladder-71d53.firebaseapp.com',
  projectId: 'cardladder-71d53',
  ...
});
```

This key does not rotate. Security comes from the email/password credentials.

### Token Management Tips

- Cache the ID token in memory with a 5-minute buffer before expiry (refresh at 55 minutes, not 60)
- Encrypt the refresh token at rest — it's a long-lived credential
- Firebase may return a new refresh token on refresh calls — always store the latest one
- Use a mutex or similar to prevent concurrent token refreshes

## Cloud Run Search API

Base URL: `https://search-zzvl7ri3bq-uc.a.run.app/search`

All requests are GET with query parameters. All require `Authorization: Bearer {idToken}`.

### Indexes

| Index | Purpose | Key filters |
|-------|---------|-------------|
| `collectioncards` | User's collection | `collectionId`, `hasQuantityAvailable` |
| `salesarchive` | Historical sale comps | `gemRateId`, `condition`, `gradingCompany` |
| `cards` | Card Ladder's card catalog | `set`, `condition` |

### Fetch Collection Cards

```
GET /search?index=collectioncards
  &query=
  &page=0
  &limit=100
  &filters=collectionId:{collectionId}|hasQuantityAvailable:true
  &sort=player
  &direction=asc
```

Response:
```json
{
  "hits": [
    {
      "collectionCardId": "MqOpo9AvkzPhQS4vEHDa",
      "collectionId": "4ROuX7h...",
      "category": "Pokemon",
      "condition": "PSA 8",
      "year": "2023",
      "number": "198",
      "set": "Pokemon Mew En-151",
      "variation": "Special Illustration Rare",
      "label": "2023 Pokemon Mew En-151 Venusaur Ex Special Illustration Rare #198 PSA 8",
      "player": "Venusaur Ex",
      "image": "https://d1htnxwo4o0jhw.cloudfront.net/cert/193222677/...",
      "imageBack": "https://d1htnxwo4o0jhw.cloudfront.net/cert/193222677/...",
      "currentValue": 133.5,
      "investment": 123,
      "profit": 10.5,
      "weeklyPercentChange": 0,
      "monthlyPercentChange": 0,
      "dateAdded": "2026-03-24T17:38:34.645Z",
      "hasQuantityAvailable": true,
      "sold": false
    }
  ],
  "totalHits": 342
}
```

**Important**: Collection cards do NOT include `gemRateId`. You need the Firestore lookup (see below) to get it.

**Pagination**: Increment `page` until `hits` accumulated >= `totalHits` or a page returns fewer than `limit` results.

**Cert number extraction**: The PSA cert number can be extracted from the image URL path using regex: `/cert/(\d+)/`

### Fetch Sales Comps

Requires `gemRateId` (obtained from Firestore) and the condition in `g{grade}` format.

```
GET /search?index=salesarchive
  &query=
  &limit=100
  &filters=condition:g8|gemRateId:{gemRateId}|gradingCompany:psa
  &sort=date
```

Response:
```json
{
  "hits": [
    {
      "itemId": "ebay-257399448670",
      "date": "2026-03-11T00:33:00.000Z",
      "price": 120,
      "platform": "eBay",
      "listingType": "BestOffer",
      "seller": "panthertrading1",
      "feedback": 600,
      "url": "https://www.ebay.com/itm/257399448670",
      "slabSerial": "124801974",
      "cardDescription": "2023 Pokemon Mew EN-151 Venusaur EX Special Illustration Rare 198",
      "gemRateId": "c20259c57c80b35a3d9d4d0666438c901e3edb1a",
      "condition": "g8",
      "gradingCompany": "psa"
    }
  ],
  "totalHits": 47
}
```

### Condition Format

The search API uses two different condition formats:

| Context | Format | Example |
|---------|--------|---------|
| Collection cards | Full label | `PSA 8`, `PSA 9.5` |
| Sales comps filter | Short code | `g8`, `g9.5`, `g10` |
| Firestore documents | Short code | `g8`, `g9.5`, `g10` |

When querying sales comps, use the `gemRateCondition` from Firestore (e.g., `g8`), not the collection card's `condition` field (e.g., `PSA 8`).

### Rate Limiting

The search API doesn't document rate limits, but in practice 1 request/second is safe. Implement a rate limiter to avoid being blocked.

## Firestore REST API

Firestore contains detailed card documents that the search API doesn't expose — most importantly `gemRateId` and `gemRateCondition`.

### Document Path

```
projects/cardladder-71d53/databases/(default)/documents/
  users/{uid}/collections/{collectionId}/collection_cards/{collectionCardId}
```

- `{uid}`: Firebase UID from the login response (`localId` field)
- `{collectionId}`: Your Card Ladder collection ID
- `{collectionCardId}`: From the search API's `collectionCardId` field

### Read a Single Document

```
GET https://firestore.googleapis.com/v1/projects/cardladder-71d53/databases/(default)/documents/users/{uid}/collections/{collectionId}/collection_cards/{cardId}
Authorization: Bearer {idToken}
```

### List All Collection Cards

```
GET https://firestore.googleapis.com/v1/projects/cardladder-71d53/databases/(default)/documents/users/{uid}/collections/{collectionId}/collection_cards
  ?pageSize=100
  &pageToken={nextPageToken}   // omit for first page
Authorization: Bearer {idToken}
```

Paginate by following `nextPageToken` in the response until it's absent.

### Document Structure

Firestore REST API returns fields in a typed format:

```json
{
  "name": "projects/cardladder-71d53/.../collection_cards/V5ZwnMh9iLOASoMyCZc8",
  "fields": {
    "gemRateId": { "stringValue": "c20259c57c80b35a3d9d4d0666438c901e3edb1a" },
    "gemRateCondition": { "stringValue": "g8" },
    "slabSerial": { "stringValue": "137354056" },
    "label": { "stringValue": "2023 Pokemon Mew En-151 Venusaur Ex ..." },
    "player": { "stringValue": "Venusaur Ex" },
    "set": { "stringValue": "Pokemon Mew En-151" },
    "condition": { "stringValue": "PSA 8" },
    "number": { "stringValue": "198" },
    "year": { "stringValue": "2023" },
    "currentValue": { "integerValue": "135" },
    "investment": { "integerValue": "123" },
    "pop": { "integerValue": "7939" },
    "gradingCompany": { "stringValue": "psa" },
    "category": { "stringValue": "Pokemon" },
    "collectionCardId": { "stringValue": "V5ZwnMh9iLOASoMyCZc8" },
    "collectionId": { "stringValue": "4ROuX7h0KsZOcenGKsQX" },
    "uid": { "stringValue": "xzl4x..." },
    "image": { "stringValue": "https://d1htnxwo4o0jhw.cloudfront.net/cert/..." },
    "sold": { "booleanValue": false },
    "hasQuantityAvailable": { "booleanValue": true }
  },
  "createTime": "2026-03-24T17:38:36.052045Z",
  "updateTime": "2026-03-29T12:29:47.743371Z"
}
```

### Key Fields

| Field | Type | Description |
|-------|------|-------------|
| `gemRateId` | string | Unique card identifier (SHA-1 hash, 40 hex chars). Used to query sales comps. |
| `gemRateCondition` | string | Grade in short format (`g8`, `g10`). Used as filter for sales comps. |
| `slabSerial` | string | PSA cert number. Links to your inventory. |
| `pop` | integer | PSA population count for this card+grade. |
| `currentValue` | integer | Card Ladder's current valuation (dollars, not cents). |
| `collectionCardId` | string | Unique ID within the collection. |

## Recommended Sync Flow

```
1. AUTHENTICATE
   Login → get idToken + refreshToken + uid
   Store refreshToken encrypted, cache idToken

2. FETCH COLLECTION (Search API)
   FetchAllCollection(collectionId) → []CollectionCard
   Paginate 100/page until all fetched

3. FETCH CARD DETAILS (Firestore REST)
   List all documents at collection_cards path
   Paginate 100/page
   Extract gemRateId + gemRateCondition for each card
   Build map: collectionCardId → {gemRateId, gemRateCondition}

4. MATCH TO YOUR INVENTORY
   For each collection card:
     - Match by cert number (extracted from image URL or slabSerial)
     - Or match by image URL
     - Store the mapping: cert → {collectionCardId, gemRateId, gemRateCondition}

5. UPDATE VALUES
   For matched cards, update your records with CL's currentValue

6. FETCH SALES COMPS (Search API)
   For each mapped card with a gemRateId:
     FetchSalesComps(gemRateId, gemRateCondition, "psa")
     Store/upsert by (gemRateId, itemId) composite key

7. REPEAT DAILY
   Steps 2-6 run on a schedule (e.g., 4 AM UTC)
   Token refreshes automatically when expired
```

## Finding Your Collection ID

Log into Card Ladder's web app and inspect the network tab. Look for requests to:
```
https://search-zzvl7ri3bq-uc.a.run.app/search?index=collectioncards&...&filters=collectionId:{YOUR_ID}|...
```

The `collectionId` value in the filter is your collection ID.

## Finding the Firebase API Key

Inspect Card Ladder's web app source. Search the JavaScript for `firebase.initializeApp` or look for a string starting with `AIza`. The `apiKey` field is what you need.

Alternatively, watch the network tab for requests to `identitytoolkit.googleapis.com` or `securetoken.googleapis.com` — the `key=` query parameter is the API key.
