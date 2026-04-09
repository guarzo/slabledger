---
name: new-handler
description: Add a new HTTP API endpoint following the hexagonal architecture pattern
---

# New HTTP Handler

## When to Use

Use this skill when adding a new API endpoint to the application.

## Architecture Overview

Handlers live in `internal/adapters/httpserver/handlers/`. Each handler struct groups related endpoints (e.g., `FavoritesHandlers` for all favorites routes). Routes are registered in `internal/adapters/httpserver/routes.go`. The handler layer is an adapter — it translates HTTP into domain service calls.

## Steps

### Step 1: Decide Where the Handler Lives

- **Existing domain** (e.g., campaigns, pricing, social): Add a method to the existing handler struct in the appropriate file
- **New domain**: Create a new handler file at `internal/adapters/httpserver/handlers/{domain}.go`

### Step 2: Define Request/Response Types

Define types in the handler file (or in a shared types file if reused):

```go
type createThingRequest struct {
    Name   string `json:"name"`
    Amount int    `json:"amountCents"`
}

type createThingResponse struct {
    ID   int    `json:"id"`
    Name string `json:"name"`
}
```

**Convention:** API responses use dollars (USD) for monetary values. The backend stores cents. Convert at the handler boundary.

### Step 3: Implement the Handler Method

```go
func (h *ThingHandler) HandleCreateThing(w http.ResponseWriter, r *http.Request) {
    ctx := r.Context()

    // Parse request body
    var req createThingRequest
    if !decodeBody(w, r, &req) {
        return
    }

    // Call domain service
    result, err := h.service.CreateThing(ctx, req.Name, req.Amount)
    if err != nil {
        if errors.Is(err, domain.ErrNotFound) {
            writeError(w, http.StatusNotFound, "Thing not found")
            return
        }
        h.logger.Error(ctx, "failed to create thing", observability.Err(err))
        writeError(w, http.StatusInternalServerError, "Internal server error")
        return
    }

    writeJSON(w, http.StatusCreated, createThingResponse{
        ID:   result.ID,
        Name: result.Name,
    })
}
```

**Key patterns:**

- Always extract `ctx` from request
- Use `decodeBody(w, r, &req)` helper for JSON parsing (returns false on error, writes 400 automatically)
- Use `requireUser(w, r)` for authenticated endpoints (returns nil on auth failure)
- Call domain service — never access DB directly from handlers
- Use `writeError()` and `writeJSON()` helpers from `handlers/helpers.go`
- Map domain sentinel errors to HTTP status codes (400, 404, 409, etc.)
- Log errors with structured logging: `h.logger.Error(ctx, "msg", observability.Err(err))`

### Step 4: Register the Route

Add the route in `internal/adapters/httpserver/routes.go`:

```go
// In the appropriate route group
authRoute(mux, "POST /api/things", thingHandler.HandleCreateThing, authMid)
```

**Route registration functions:**

- `authRoute()` — requires authentication (most endpoints)
- `adminRoute()` — requires admin role
- `publicRoute()` — no auth required (health, login, static)
- Path parameters use `{name}` syntax: `/api/things/{id}`
- Extract path params with `r.PathValue("id")`

### Step 5: Wire Dependencies

If creating a **new handler struct**, wire it in `cmd/slabledger/main.go`:

1. Create the handler instance: `thingHandler := handlers.NewThingHandler(thingService, logger)`
2. Pass it to the router setup

If adding a method to an **existing handler**, no wiring changes needed.

### Step 6: Add Frontend Types

If the endpoint will be called from the React frontend:

1. Add TypeScript interfaces in `web/src/types/` matching the Go JSON tags
2. Add the API call in the appropriate file under `web/src/js/api/`
3. Add a React Query hook in `web/src/react/queries/` if the UI needs it

### Step 7: Update API Documentation

Add the endpoint to `docs/API.md` with method, path, request body, response shape, auth requirement, and an example curl.

### Step 8: Write Tests

Create or extend handler tests in `internal/adapters/httpserver/handlers/{domain}_test.go`:

```go
func TestHandleCreateThing(t *testing.T) {
    tests := []struct {
        name       string
        body       string
        setupMock  func(*mocks.MockThingService)
        wantStatus int
    }{
        {
            name: "success",
            body: `{"name":"test","amountCents":100}`,
            setupMock: func(m *mocks.MockThingService) {
                m.CreateThingFn = func(ctx context.Context, name string, amount int) (*Thing, error) {
                    return &Thing{ID: 1, Name: name}, nil
                }
            },
            wantStatus: http.StatusCreated,
        },
        {
            name: "invalid body",
            body: `{invalid`,
            wantStatus: http.StatusBadRequest,
        },
    }
    for _, tc := range tests {
        t.Run(tc.name, func(t *testing.T) {
            // ... table-driven test execution
        })
    }
}
```

Use mocks from `internal/testutil/mocks/` — never create inline mocks. See `internal/testutil/mocks/README.md` for the Fn-field pattern.

## Checklist

- [ ] Handler method follows: parse → validate → call domain → respond
- [ ] Route registered in `routes.go` with correct auth level
- [ ] Request/response types have proper JSON tags
- [ ] Domain errors mapped to appropriate HTTP status codes
- [ ] Cents-to-dollars conversion at handler boundary (if monetary)
- [ ] Dependencies wired in `main.go` (if new handler struct)
- [ ] Frontend types synced (if frontend calls this endpoint)
- [ ] `docs/API.md` updated
- [ ] Tests written with table-driven pattern and mocks from `internal/testutil/mocks/`

## Reference

See `references/example-handler.md` for a worked example based on the Favorites handler.
