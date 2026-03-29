# SlabLedger

[![CI](https://github.com/guarzo/slabledger/actions/workflows/test.yml/badge.svg)](https://github.com/guarzo/slabledger/actions/workflows/test.yml)
[![Frontend CI](https://github.com/guarzo/slabledger/actions/workflows/frontend-ci.yml/badge.svg)](https://github.com/guarzo/slabledger/actions/workflows/frontend-ci.yml)
[![Docker Build](https://github.com/guarzo/slabledger/actions/workflows/docker-build.yml/badge.svg)](https://github.com/guarzo/slabledger/actions/workflows/docker-build.yml)

Track PSA Direct Buy campaigns, manage card inventory across multiple sell channels, and analyze profitability with market direction signals.

## Quick Start

```bash
# Clone and build
git clone https://github.com/guarzo/slabledger.git
cd slabledger
go build -o slabledger ./cmd/slabledger

# Set required API tokens
export PRICECHARTING_TOKEN="your_token"

# Start the web interface
./slabledger
# Open http://localhost:8081
```

## Features

- **Campaign Management** - Create and configure PSA Direct Buy campaigns with buy parameters, daily spend caps, and fee settings
- **Purchase & Sale Tracking** - Record card purchases with PSA cert numbers, sell through eBay, TCGPlayer, local (GameStop), or other channels
- **CSV Import** - Bulk import purchases from PSA CSV exports with automatic grade extraction
- **P&L Analytics** - Campaign-level and channel-level profit/loss, ROI, sell-through rate, and average days-to-sell
- **Inventory Aging** - Track unsold cards with days held and market direction signals (rising/falling/stable)
- **Market Signals** - Compare real-time sold prices against Card Ladder valuations to inform sell-channel decisions
- **Multi-Source Price Fusion** - Aggregates CardHedger and PriceCharting for accurate graded pricing
- **Card Pricing** - Look up current prices across all grades and sources
- **Favorites** - Save and track cards of interest
- **Archive** - Soft-delete campaigns while preserving history
- **API Status Dashboard** - Monitor per-provider API usage, success rates, and latency

## How It Works

PSA sources already-graded cards through automated campaigns. You configure buy parameters (CL%, grade range, spend cap), then track purchases and sales through multiple channels:

| Channel | Economics |
|---------|-----------|
| **eBay** | Market price minus ~12.35% fees |
| **TCGPlayer** | Online marketplace, similar fee structure |
| **Local** (GameStop, card shows) | 90% of Card Ladder value, no listing fees |
| **Other** | Website sales, direct sales, etc. |

The system compares real-time sold data (from PriceCharting/fusion) against Card Ladder valuations to reveal market direction, helping you choose the optimal sell channel.

## Stack

Go backend with SQLite, React + Radix UI + TanStack Query + Vite + Tailwind frontend.

## Requirements

- Go 1.25.2+
- [PriceCharting API token](https://www.pricecharting.com/api) (required)
- Node.js 18+ (for frontend development)

## Environment Variables

```bash
# Required
PRICECHARTING_TOKEN="..."    # Graded prices and sales data

# Optional
CARD_HEDGER_API_KEY="..."    # Supplementary pricing (unlimited plan)
LOG_LEVEL="info"             # debug, info, warn, error
```

## API Endpoints

### Campaigns
| Method | Path | Description |
|--------|------|-------------|
| GET | `/api/campaigns` | List campaigns (`?includeArchived=true`) |
| POST | `/api/campaigns` | Create campaign |
| GET | `/api/campaigns/{id}` | Get campaign |
| PUT | `/api/campaigns/{id}` | Update campaign |
| POST | `/api/campaigns/{id}/archive` | Archive campaign |
| GET/POST | `/api/campaigns/{id}/purchases` | List/create purchases |
| POST | `/api/campaigns/{id}/purchases/import` | CSV import (multipart) |
| GET/POST | `/api/campaigns/{id}/sales` | List/create sales |
| GET | `/api/campaigns/{id}/pnl` | Campaign P&L summary |
| GET | `/api/campaigns/{id}/pnl-by-channel` | P&L by sale channel |
| GET | `/api/campaigns/{id}/fill-rate` | Daily spend vs cap |
| GET | `/api/campaigns/{id}/days-to-sell` | Distribution histogram |
| GET | `/api/campaigns/{id}/inventory` | Unsold cards + aging + market signals |
| GET | `/api/campaigns/cash-flow` | Overall cash position |

### Other
| Method | Path | Description |
|--------|------|-------------|
| GET | `/api/sets` | List Pokemon sets |
| GET | `/api/cards/search` | Search cards |
| GET | `/api/health` | Health check |
| GET/POST/DELETE | `/api/favorites` | Manage favorites |
| GET | `/api/status/api-usage` | API usage per provider |

## Testing

```bash
go test ./...                              # Run all tests
go test -race -timeout 10m ./...           # With race detection
```

## Frontend Development

```bash
cd web
npm install
npm run dev      # Development server on :5173
npm run build    # Production build
npm test         # Run tests
```

Key frontend libraries: React, TanStack React Query, Radix UI, TanStack React Virtual, Tailwind CSS

## Documentation

- [User Guide](docs/USER_GUIDE.md) - How to use the application
- [Architecture](docs/ARCHITECTURE.md) - System design and key decisions
- [Development](docs/DEVELOPMENT.md) - Caching, rate limiting, API integrations
- [Next Steps](docs/NEXT_STEPS.md) - Planned features

## License

MIT
