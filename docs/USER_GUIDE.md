# Graded Card Portfolio Tracker - User Guide

A guide to managing PSA Direct Buy campaigns, tracking purchases and sales across multiple channels, and analyzing profitability.

## Table of Contents

1. [Overview](#overview)
2. [Getting Around](#getting-around)
3. [Campaigns](#campaigns)
   - [Creating a Campaign](#creating-a-campaign)
   - [Campaign Settings](#campaign-settings)
   - [Archiving Campaigns](#archiving-campaigns)
4. [Adding Cards](#adding-cards)
   - [Cert Intake](#cert-intake)
   - [PSA Sync](#psa-sync)
   - [CSV Import](#csv-import)
5. [Sales](#sales)
   - [Recording Sales](#recording-sales)
   - [Sale Channels](#sale-channels)
   - [Fee Calculation](#fee-calculation)
6. [Analytics](#analytics)
   - [P&L Summary](#pl-summary)
   - [Channel Breakdown](#channel-breakdown)
   - [Fill Rate](#fill-rate)
   - [Days to Sell](#days-to-sell)
   - [Cash Flow](#cash-flow)
7. [Inventory & Market Signals](#inventory--market-signals)
   - [Inventory Aging](#inventory-aging)
   - [Market Direction](#market-direction)
   - [Sell Channel Recommendations](#sell-channel-recommendations)
8. [Card Pricing](#card-pricing)
9. [API Status](#api-status)
10. [FAQ](#faq)

---

## Overview

This application tracks PSA Direct Buy campaigns where PSA sources already-graded cards for resale through multiple channels. The core workflow is:

1. **Create a campaign** with buy parameters (CL%, grade range, daily spend cap)
2. **Add cards** as they are acquired — by cert number, via PSA sync, or by CSV import
3. **Record sales** through eBay, the website, or in person
4. **Analyze profitability** with P&L dashboards, channel comparisons, and market signals

---

## Getting Around

The header navigation has five destinations:

| Page | Path | What it is for |
|------|------|----------------|
| **Dashboard** | `/` | Portfolio-wide stats, top performers, weekly review |
| **Campaigns** | `/campaigns` | Create, edit and archive campaigns; per-campaign P&L; invoices |
| **Inventory** | `/inventory` | Every unsold card, with pricing, market signals and row actions |
| **Scan** | `/scan` | Cert lookup and card intake |
| **Invoices** | `/invoices` | Capital position and due dates |

Admins also see **Admin** (`/admin`), which holds integrations, API health and user
management. There is no per-campaign detail page — campaign rows expand in place on
the Campaigns page, and all card-level work happens on **Inventory**.

---

## Campaigns

### Creating a Campaign

Click **+ New Campaign** on the Campaigns page. The form is grouped into three
sections.

**Identity**

| Field | Description | Example |
|-------|-------------|---------|
| **Name** | Campaign identifier (required) | "Vintage Core PSA 8-9" |
| **Phase** | Lifecycle phase | Draft |
| **Year Range** | Target years | "1999-2003" |

**Targeting**

| Field | Description | Example |
|-------|-------------|---------|
| **Grade Range** | Target PSA grades (slider) | 8–9 |
| **Price Range** | Target price range | "250-1500" |
| **Languages** | Multi-select language axis. **Leaving it empty casts an open net** — it does not exclude everything | English, Japanese |
| **Subject Mode** | Whether the subject list targets or excludes | Target / Exclude |
| **Targeted Subjects** / **Excluded Subjects** | Subject list; the label follows Subject Mode | "charizard", "pikachu" |
| **Denied Specs** | Read-only. Portal-managed — add or remove these in the PSA portal. Only shown when the campaign has some | — |

**Economics**

| Field | Description | Example |
|-------|-------------|---------|
| **Buy Terms (%)** | Buy at this percentage of Card Ladder value | 78 |
| **Daily Spend Cap ($)** | Maximum daily spend | 500 |
| **Expected Fill Rate (%)** | Share of eligible offers you expect to win | 80 |
| **eBay Fee %** | Marketplace fee applied to eBay sales | 12.35 |
| **PSA Sourcing Fee ($)** | Per-card sourcing fee | 3.00 |
| **CL Confidence** | Minimum confidence, rated 1–5 | 3 |

The campaign starts in **Draft** phase. Change to **Active** when you start buying.

### Campaign Settings

Edit a campaign in place from its row on the Campaigns page. The edit form exposes the
same Identity, Targeting and Economics fields as creation.

### Archiving Campaigns

Archiving soft-deletes a campaign. It remains in the database but is hidden from the default campaign list. Toggle **Show archived** on the campaigns page to view archived campaigns.

---

## Adding Cards

Cards enter inventory by certificate number, not by typing card details. There are
three routes in.

### Cert Intake

The main path. Go to **Scan**, then scan or type cert numbers into the intake field
("Scan or type cert number…"). Submitting the batch sends the certs to the server,
which looks each one up with PSA and creates the purchase from the returned card
data — card name, number, year, grade and population all come from PSA rather than
from you. The set name is only adopted when PSA returns a real set; generic values
like "TCG Cards" are left blank for later enrichment.

Two things cert intake deliberately does *not* do:

- **It does not capture cost.** Buy cost, CL value and sourcing fee are all created
  at zero. Fill them in afterwards from **Inventory** if you need accurate P&L.
- **It does not pick a campaign.** Scanned cards land in a fixed no-campaign bucket
  rather than being attributed to a campaign nobody chose.

Certs already in the system are reported as already-existing rather than duplicated,
and any that have already sold are called out separately in the result.

### PSA Sync

Admins can pull PSA orders directly. Under **Admin → Integrations**, the PSA sync
surfaces incoming items as **pending items**, which you then assign to a campaign or
dismiss. Unassigned items stay pending rather than landing in a campaign silently.

### CSV Import

Two CSV formats are accepted, and they are not interchangeable.

**Orders CSV (sales) — available in the UI.** Under **Admin → Integrations → Import
Sales**, click **Upload Orders CSV**. This matches sales against existing inventory by
PSA cert number; it does not create purchases. The parser requires all six of these
columns, matched case-insensitively by header name:

`date`, `sales channel`, `product title`, `grading company`, `cert number`, `unit price`

An optional `order` column is used for the order number and an optional `grade` column
for the grade. After upload you get a **Review Import** table and confirm the matches
before any sale is written.

**PSA export CSV (purchases) — API only.** `POST /api/purchases/import-psa` accepts a
PSA communication spreadsheet. There is currently no button for this in the shipped UI.
The parser scans the first six rows for a header row and needs at least three of these
four columns to recognize it:

`cert number`, `listing title`, `grade`, `price paid`

If no such header row is found the import fails outright with
`could not find PSA header row` — nothing is imported. Rows missing a price or a grade
are **skipped**, not defaulted: when the `grade` column is empty the grade is recovered
from the listing title (e.g. "Charizard PSA 9" → 9), and a row that still has no grade
is reported as `skipped: missing price or grade`.

The import also skips duplicate certificate numbers, applies the campaign's sourcing
fee, and reports per-row results.

---

## Sales

### Recording Sales

On the **Inventory** page, open a card's **Actions** menu and choose **Sell**. Select
multiple cards first to record a batch sale in one pass.

1. Choose the sale channel
2. Enter the sale price
3. Enter the sale date

The system automatically computes:
- **Sale fee** based on channel and campaign fee settings
- **Days to sell** (sale date minus purchase date)
- **Net profit** (sale price - buy cost - sourcing fee - sale fee)

Sales can also arrive in bulk through the orders CSV import — see
[CSV Import](#csv-import).

### Sale Channels

There are three active channels:

| Channel | Description | Fees |
|---------|-------------|------|
| **eBay** | eBay marketplace | Campaign's eBay fee % (default 12.35%) |
| **Website** | Online store / direct sales | 3% card processing |
| **In Person** | Card shows, in-person sales | No fees |

Older records may carry **legacy** channel values — `tcgplayer`, `local`, `other`,
`gamestop`, `cardshow`, `doubleholo`. These are still readable and are normalized for
display, analytics and fees: `tcgplayer` folds into **eBay**, and every other legacy
value folds into **In Person**.

### Fee Calculation

- **eBay**: `salePriceCents * campaign.ebayFeePct`, rounded to the nearest cent. A
  campaign fee of 0, negative, or ≥ 100% is treated as unset and falls back to 12.35%.
- **Website**: `salePriceCents * 3%`, rounded to the nearest cent.
- **In Person**: $0 (the discount is already baked into the sale price)
- **Net Profit**: `salePrice - buyCost - sourcingFee - saleFee`

---

## Analytics

Portfolio-level analytics live on the **Dashboard**, which shows a hero stats bar, top
performers and a weekly review. Per-campaign figures appear as columns on each campaign
row on the **Campaigns** page: **P&L**, **ROI**, **Sell-through**, and **Cap · Buy%**
(the daily spend cap and the percent of CL value paid on incoming buys, suppressed for
closed campaigns).

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

The **Inventory** page shows all unsold cards. Columns are **Card**, **Gr** (grade),
**Cost**, **List / Rec** (listed price and recommendation), **P/L**, **Status** and
**Actions**; every column except Actions sorts. Expanding a row reveals the cert
number, the Card Ladder value recorded at purchase, and days held.

Cards held longer than 30 days are treated as deeply stale in the sell signals.

### Show Preparation

Use inventory's price-support controls to find cards whose recent sales support
their sticker price, then add selected slabs to a named show shortlist. Saved
lists are available at `/shows` and retain saved membership and packing checks
across reloads. Unsaved checkbox selection is local to the current page.

Price support compares the **last-synced DH listing price** with exact-match sales
from the last 30 UTC calendar dates, including today:

| Status | Meaning |
|---|---|
| **Supported** | At least two matching sales, with a median at least 90% of listed price. |
| **Thin evidence** | One matching sale meets that threshold. |
| **Below target** | The median is below the threshold. |
| **No recent comps** | A current, complete lookup found no matching sales. |
| **Needs review** | Matching, price association, freshness, or coverage is uncertain. |
| **No listed price** | No known positive DH listing price is available. |

For example, a $300 listing needs a median of at least $270. All eligible matching
sales count, including lower sales; the tool does not cherry-pick supporting comps.
Open the evidence details to see sale dates, amounts, platforms, source links,
and the actual date range. An unavailable or partial lookup is not proof that a
card has no recent sales. **Not checked**, **Checking**, stale/interrupted and failed
checks are distinct from completed support results. A fresh complete zero-sale
lookup alone establishes **No recent comps**. In show selection or a Support-filtered
view, the compact indicator names the evaluated **DH listed** price on both desktop
and mobile. It does not support a different local reviewed price or CL/Market
valuation; those ordinary inventory values keep their existing meaning. Missing
and unverified DH prices stay explicitly labeled.

Choose **Supported** even on a new installation, or open **Show selection**, to
start bounded comp checking. No card selection or saved list is needed first.
Checking uses your campaign/inventory, search, inventory tab, price band and
availability scope before applying Support filtering. The compact readiness line
counts the check scope separately from matching rows and selected slabs. Ordinary
inventory browsing and opening evidence details do not fetch source comps.

Automatic checking skips identities already current and cards without a positive
DH price. Missing-price cards remain manually checkable through **Check selected**.
One tab runs one request at a time, coalescing copies of the same card/grade: up to
10 IDs/request, 200 automatic identities and 20 requests per UTC date, and five
minutes per run. Navigating within the app preserves the current session's budget,
stops, and pending-write exclusion. Reload or a change of authenticated identity
starts a new client lifetime; multiple tabs have separate limits. These are not a
global provider quota.

Use **Cancel checking** to stop. Failed/partial checks, cancellation, time limits
and budget stops offer explicit **Retry / Continue checking** rather than retrying
on their own. Retained sales may still be shown but are not a verified current
window. A cancelled request may already have saved data; the page rereads it and
does not promise rollback. Retry after resolving the source problem or when ready
to continue. An interrupted server attempt becomes retryable after a read at its
120-second boundary, not an automatic source retry. On a packing list, **Retry
refresh** repeats the failed selection from that list, even if you change selection.
Switching to another list does not offer or retarget that previous-list retry.

At UTC midnight, or earlier freshness expiry, the mounted workflow rereads status
and renews eligible evidence within the same limits. Returning to a hidden tab
performs one reread, not a catch-up loop. Old sales remain inspectable while stale;
a green badge is not authoritative forever. Evaluation reads have a full-response
30-second timeout per attempt. A stalled or failed read ends with unavailable
status and **Retry price support read**, rather than checking forever. This retry
reads stored evaluations; it is not a source-refresh replay.

If CardLadder is configured for the first time while the server is running,
restart the server once to enable show-preparation refresh. Already-configured
installations require no additional setup.

Show selection starts with received, unsold, non-refunded cards from non-closed
campaigns. Unreceived cards can be included as planning candidates, but cannot be
packed. Paused campaigns remain eligible. You can deliberately bring a physically
eligible card with weak evidence; a support label is advice, not a packing rule.

Selecting a card pauses subsequent automatic batches. The active batch may finish.
If selected data changes, rows stay identifiable and Add is blocked until you
explicitly review/reselect; the application never silently acknowledges new prices
or evidence. **Reveal selected** recovers cards outside your current view. Clearing
selection resumes eligible work unless you cancelled or reached a stop condition.
Closing or hiding an inline sale editor releases its presentation pause, but any
submitted sale, price, hint, or DH-match request still blocks checking until it
settles, including after navigation.

The contextual selection bar appears only after selection. Choose **Add selected
to show** to reveal a destination, then choose a saved list or name a new one.
Escape closes destination details before clearing selection. Successful Add links
to the submitted packing list. The empty **Shows** page offers one create-first path;
creating a list alone does not select or check inventory.

On the saved list, check **Packed** as you load each slab. Price/support changes
are flagged for review rather than silently acknowledged. A price change after
packing is a reminder to check the physical sticker. Sold, refunded, removed, or
closed-campaign members stay visible with their packing history; they are excluded
from available value and cannot be newly packed. You can still unpack or remove
them explicitly.

Known listed-value totals exclude missing and ambiguous DH prices rather than
substituting CardLadder values. Adding or packing cards **does not** change prices,
delist online inventory, reserve a card, or record a sale. Continue recording sales
through the existing sales workflow. **Update list status** rereads saved prices
and availability; source acquisition on that page is an explicit comp check.

Local verification uses a committed real-browser/Go/PostgreSQL upgrade fixture,
not preseeded show evidence. Maintainer reproduction and isolated two-tab preview
instructions are in [the real-wire test guide](../web/tests/show-readiness-real.md).
Production rollout and controlled missing/stale-plus-fresh verification still
require separate operator authorization.

### Market Direction

For each unsold card, the system compares the most recent sold price against the Card Ladder valuation recorded at purchase:

| Direction | Meaning | Delta |
|-----------|---------|-------|
| **Rising** | Market price above CL valuation | ≥ +5% |
| **Falling** | Market price below CL valuation | ≤ -5% |
| **Stable** | Market price near CL valuation | strictly inside ±5% |

A drift of exactly ±5% counts as rising or falling, not stable.

### Sell Channel Recommendations

Based on market direction:

| Signal | Recommendation |
|--------|----------------|
| **Rising** | Consider eBay — market is ahead of trailing valuations |
| **Falling** | Consider in-person — lock in before valuations drop |
| **Stable** | Either channel — in-person for speed, eBay for margin |

**Key insight**: Card Ladder valuations are a trailing indicator. When real-time sold prices diverge from CL, it reveals market direction before CL updates.

---

## Card Pricing

There is no standalone Pricing page — `/pricing` redirects to the Dashboard. Pricing
now lives where you act on it, on the **Inventory** page:

- The **List / Rec** column shows the current listed price and the recommended price
- **Set Price** on a row's Actions menu overrides the price
- **Fix Pricing** resolves a card whose pricing could not be determined
- **Scan** (`/scan`) looks up a single cert without adding it to inventory

---

## API Status

The status page shows real-time API usage for each pricing provider:

| Provider | Daily Limit | Description |
|----------|-------------|-------------|
| **DH (DoubleHolo)** | Enterprise plan | Graded pricing, market data, sales history |

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

Card Ladder is a valuation service that provides market values for graded cards. Their
values drive both the PSA buy price (campaign CL%) and the in-person sell price.

CL values are fetched automatically. The application has a Card Ladder client and a
refresh scheduler that writes updated valuations back onto purchases, so you do not
enter them by hand. Admins can drive it from **Admin → Integrations**: save credentials,
check status and coverage, review failures, trigger a refresh, add a card, or sync back
to Card Ladder.

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

*Last updated: August 2026*
