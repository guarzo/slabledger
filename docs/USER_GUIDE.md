# Graded Card Portfolio Tracker - User Guide

A guide to managing PSA Direct Buy campaigns, tracking purchases and sales across multiple channels, and analyzing profitability.

## Table of Contents

1. [Overview](#overview)
2. [Campaigns](#campaigns)
   - [Creating a Campaign](#creating-a-campaign)
   - [Campaign Settings](#campaign-settings)
   - [Archiving Campaigns](#archiving-campaigns)
3. [Purchases](#purchases)
   - [Adding Purchases](#adding-purchases)
   - [CSV Import](#csv-import)
4. [Sales](#sales)
   - [Recording Sales](#recording-sales)
   - [Sale Channels](#sale-channels)
   - [Fee Calculation](#fee-calculation)
5. [Analytics](#analytics)
   - [P&L Summary](#pl-summary)
   - [Channel Breakdown](#channel-breakdown)
   - [Fill Rate](#fill-rate)
   - [Days to Sell](#days-to-sell)
   - [Cash Flow](#cash-flow)
6. [Inventory & Market Signals](#inventory--market-signals)
   - [Inventory Aging](#inventory-aging)
   - [Market Direction](#market-direction)
   - [Sell Channel Recommendations](#sell-channel-recommendations)
7. [Card Pricing](#card-pricing)
8. [Favorites](#favorites)
9. [API Status](#api-status)
10. [FAQ](#faq)

---

## Overview

This application tracks PSA Direct Buy campaigns where PSA sources already-graded cards for resale through multiple channels. The core workflow is:

1. **Create a campaign** with buy parameters (CL%, grade range, daily spend cap)
2. **Record purchases** as cards are acquired (manually or via CSV import)
3. **Record sales** through eBay, TCGPlayer, local (GameStop/card shows), or other channels
4. **Analyze profitability** with P&L dashboards, channel comparisons, and market signals

The system also provides card pricing lookup and favorites tracking.

---

## Campaigns

### Creating a Campaign

Click **+ New Campaign** on the Campaigns page. Fill in:

| Field | Description | Example |
|-------|-------------|---------|
| **Name** | Campaign identifier (required) | "Vintage Core PSA 8-9" |
| **Sport** | Card sport/category | "Pokemon" |
| **Year Range** | Target years | "1999-2003" |
| **Grade Range** | Target PSA grades | "8-9" |
| **Price Range** | Target price range | "250-1500" |
| **Buy Terms (CL %)** | Buy at this percentage of Card Ladder value | 0.78 (78%) |
| **Daily Spend Cap** | Maximum daily spend | $500.00 |
| **CL Confidence** | Minimum confidence threshold | 3.5 |
| **Inclusion List** | Target card names/sets | "charizard pikachu" |

The campaign starts in **Draft** phase. Change to **Active** when you start buying.

### Campaign Settings

On the campaign detail page, click the **Settings** tab to:

- Edit campaign parameters (name, phase, buy terms, caps, fees)
- View current fee configuration (eBay fee %, sourcing fee)
- Archive the campaign

### Archiving Campaigns

Archiving soft-deletes a campaign. It remains in the database but is hidden from the default campaign list. Toggle **Show archived** on the campaigns page to view archived campaigns.

---

## Purchases

### Adding Purchases

On the campaign detail page, go to the **Purchases** tab and click **+ Add Purchase**:

| Field | Description |
|-------|-------------|
| **Card Name** | Full card name |
| **Cert #** | PSA certificate number (unique across all campaigns) |
| **PSA Grade** | Grade 1-10 |
| **Buy Cost** | Amount paid for the card |
| **CL Value** | Card Ladder valuation at time of purchase |
| **Purchase Date** | Date acquired |

### CSV Import

For bulk imports from PSA export files:

1. Go to the **Purchases** tab
2. Click **Choose CSV File**
3. Upload a CSV with three columns: `Card Title`, `Price`, `Date`

The import automatically:
- Extracts PSA grades from card titles (e.g., "Charizard PSA 9" → grade 9)
- Defaults to grade 9 if no grade is found in the title
- Skips duplicate certificate numbers
- Uses the campaign's sourcing fee for all imports
- Reports import results (imported, skipped, errors)

**CSV format example:**
```csv
Card Title,Price,Date
2021 Celebrations Charizard PSA 9,500.00,2026-01-15
Pikachu VMAX PSA 10,200.00,2026-01-16
```

---

## Sales

### Recording Sales

On the **Sales** tab, click **+ Record Sale**:

1. Select the unsold card from the dropdown
2. Choose the sale channel
3. Enter the sale price
4. Enter the sale date

The system automatically computes:
- **Sale fee** based on channel and campaign fee settings
- **Days to sell** (sale date minus purchase date)
- **Net profit** (sale price - buy cost - sourcing fee - sale fee)

### Sale Channels

| Channel | Description | Fees |
|---------|-------------|------|
| **eBay** | eBay marketplace | Campaign's eBay fee % (default 12.35%) |
| **TCGPlayer** | TCGPlayer marketplace | Same as eBay fee % |
| **Local** | Card shows, GameStop, in-person | No marketplace fees |
| **Other** | Website, direct sales | No marketplace fees |

### Fee Calculation

- **eBay/TCGPlayer**: `salePriceCents * campaign.ebayFeePct` (rounded up)
- **Local/Other**: $0 (the discount is already baked into the sale price)
- **Net Profit**: `salePrice - buyCost - sourcingFee - saleFee`

---

## Analytics

Access analytics from the **Analytics** tab on the campaign detail page.

### P&L Summary

Shows overall campaign performance:

| Metric | Description |
|--------|-------------|
| **Total Spend** | Sum of all purchase costs + sourcing fees |
| **Revenue** | Sum of all sale prices |
| **Net Profit** | Revenue minus all costs and fees |
| **ROI** | Net profit / total spend |
| **Avg Days to Sell** | Average time from purchase to sale |

### Channel Breakdown

Compares performance across sale channels:
- Revenue, fees, and net profit per channel
- Average days to sell per channel
- Helps identify which channel generates best returns

### Fill Rate

Daily spend tracking over the last 30 days:
- Actual spend vs. daily cap
- Fill rate percentage (spend / cap)
- Number of cards purchased per day

### Days to Sell

Histogram showing how quickly cards sell:
- 0-7 days, 8-14, 15-30, 31-60, 60+
- Helps tune pricing and channel strategy

### Cash Flow

Overall position across all campaigns:
- Total unsold inventory cost
- Total unsold card count

---

## Inventory & Market Signals

### Inventory Aging

The **Inventory** tab shows all unsold cards with:
- Card name, cert number, grade
- Cost basis (buy cost + sourcing fee)
- CL value at time of purchase
- Days held since purchase

Cards held longer than 30 days are highlighted.

### Market Direction

For each unsold card, the system compares the most recent sold price (from PriceCharting/fusion sources) against the Card Ladder valuation recorded at purchase:

| Direction | Meaning | Delta |
|-----------|---------|-------|
| **Rising** | Market price above CL valuation | > +5% |
| **Falling** | Market price below CL valuation | < -5% |
| **Stable** | Market price near CL valuation | within +/-5% |

### Sell Channel Recommendations

Based on market direction:

| Signal | Recommendation |
|--------|----------------|
| **Rising** | Consider eBay/TCGPlayer — market is ahead of trailing valuations |
| **Falling** | Consider local (GameStop at 90% CL) — lock in before valuations drop |
| **Stable** | Either channel — local for speed, eBay for margin |

**Key insight**: Card Ladder valuations are a trailing indicator. When real-time sold prices diverge from CL, it reveals market direction before CL updates.

---

## Card Pricing

The **Pricing** page lets you look up current card prices across all grades and sources. Useful for:
- Checking current market value before buying or selling
- Comparing prices across PriceCharting and CardHedger
- Viewing price trends and sales history

---

## Favorites

Save cards you want to track. Requires authentication (Google OAuth).

- Add/remove favorites from the pricing page
- View all favorites on the dedicated Favorites page
- Favorites persist across sessions

---

## API Status

The status page shows real-time API usage for each pricing provider:

| Provider | Daily Limit | Description |
|----------|-------------|-------------|
| **CardHedger** | 1,000 | Secondary graded price estimates with confidence ranges |
| **PriceCharting** | No hard limit | Market data (active listings, sales velocity) |

For each provider, the page displays:
- **Calls today** with usage bar (green/amber/red)
- **Success rate** percentage
- **Average latency** in milliseconds
- **Rate limit hits** count
- **Blocked status** if the provider is temporarily unavailable

Access the status page by clicking the status indicator dot in the header.

---

## FAQ

### What is Card Ladder (CL)?

Card Ladder is a valuation service that provides market values for graded cards. Their values drive both the PSA buy price (campaign CL%) and the GameStop sell price (90% CL). Card Ladder does not offer an API, so CL values are manually entered per purchase.

### Why track CL values?

CL valuations are a trailing indicator. By comparing real-time sold data against recorded CL values, you can detect whether the market is rising or falling — which directly informs your sell-channel decision.

### How are PSA sourcing fees handled?

Each campaign has a default PSA sourcing fee (typically $3.00). This fee is added to the cost basis of each purchase and subtracted from net profit calculations.

### What happens when I archive a campaign?

The campaign and all its data (purchases, sales) are preserved. The campaign is hidden from the default list but can be viewed by enabling "Show archived". Archived campaigns cannot be modified.

### How does CSV import handle duplicates?

Certificate numbers are unique across all campaigns. If a CSV contains a cert number that already exists, that row is skipped (counted in the "Skipped" result).

### What is the status indicator in the header?

The colored dot in the header shows overall API health. Click it to view detailed per-provider usage statistics on the Status page. Green means all providers are operational, amber indicates elevated usage, and red means a provider is blocked or experiencing errors.

### What price units does the system use?

| Layer | Unit | Example |
|-------|------|---------|
| Backend/database | Cents (integer) | `50000` |
| API responses | Cents (integer) | `50000` |
| Frontend display | Dollars | `$500.00` |

The frontend converts cents to dollars for display using `(cents / 100).toFixed(2)`.

---

## Need Help?

1. Check the [Architecture Documentation](ARCHITECTURE.md) for technical details
2. Review the [Development Guide](DEVELOPMENT.md) for API integrations and caching
3. Report issues at the project repository

---

*Last updated: March 2026*
