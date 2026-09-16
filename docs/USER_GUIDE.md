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

Normal **Inventory** remains the default, with compact assessment links and the
existing sale/list controls. Choose **Price review** for a queue and persistent
price panel (focused detail with **Back to inventory list** on mobile). The queue
filters Above comps, Mixed, Limited, Supported, Unavailable and Unpriced; sorting
here does not change normal inventory ordering.

The saved assessment compares the **latest operator-committed SlabLedger asking**
(reviewed price or override, using their existing timestamp precedence) with
exact-match sales. CL valuations and the stored DH price are not substitutes.
The `recent-sales-v1` policy uses today and the previous six **UTC calendar dates**,
selects the newest five sales, and includes **all ties on the fifth sale's date**.
There may therefore be more than five qualifying sales.

| Status | Meaning |
|---|---|
| **Supported** | At least two recent sales; median at least 90% of asking, with no newest-day sale more than 20% below asking. |
| **Asking above comps** | Recent median strictly below 90% of asking. |
| **Mixed evidence** | Median clears 90%, but a newest-day sale is strictly more than 20% below asking. |
| **Limited evidence** | One recent sale, or no recent sales. Inspect the single-sale amount/gap; Limited does not mean safe. |
| **Evidence unavailable** | Identity, freshness, completeness or source health is unresolved. Retained facts are not certified current. |
| **No asking price** | No positive committed local asking, regardless of DH/CL value. |

Exactly 90% median and exactly 20% newest-day drop pass those thresholds. Half-cent
medians are compared exactly before display rounding. Older **30-day sale history**
is context only and cannot outweigh a falling recent market. Source diagnostics
retain DH amount/mismatch/association warnings and collection health separately.
Normal compact badges may say No comp data, Out of date, Data unavailable or Needs
matching to explain unavailable evidence. A failed lookup is not a zero-sale result.

Enter **Asking price** to make a read-only trial against stored evidence. The
**Trial price assessment** never replaces **Saved price assessment** or grants
packing permission. Navigation, search, filters, selection and trials do not acquire
provider comps. Preview writes no association hold, purchase, sale or list row.
Only **Save price** sends the reviewed-price commit. It can sync DH and auto-list
eligible received/PSA-shipped inventory; already-synced listed cards and ineligible
cards follow existing skip rules. DH runs asynchronously: local success is not
remote completion. Continue using explicit sale confirmation for financial sales.

Drafts stay with their card through focus/filter/mode changes while this inventory
owner is mounted. A background asking change retains the draft and warns; it does
not silently rebase your intent. Failed background inventory reads retain the last
loaded workspace. After a confirmed save with failed read-back, use **Recheck saved
state** to retry reads, not the PATCH. Saved assessment remains pending until reads
succeed. Full reload/new context does not restore unsaved drafts or selection.
`/inventory?view=pricing&review=<purchase-uuid>` deep-links to a card; unrelated
query parameters survive view changes. Mobile return restores queue focus; browser
history explicitly reopens the referenced detail.

Checkbox selection is separate from the focused editor. Select-all uses visible
queue IDs. A changed evaluation leaves the captured selected version stale and
requires explicit review/reselection before Add. An empty filter never starts a
collection job. Fleet coverage remains available in Admin, not a duplicated
readiness-heavy inventory header.

The server collects missing/due verified evidence on startup and every minute,
independently of browser activity, DH price, receipt, and CL collection membership.
`SHOW_PREP_REFRESH_ENABLED` defaults to true and is independent of CL value refresh.
Unresolved cards wait for existing verified identity enrichment. Complete zero-sale
windows are current data; yesterday's UTC window is stale until the worker renews it.

Administrators can inspect **Admin → Integrations → Show evidence** for enabled/
configured state, coverage, last finished sweep and retry/window timing. **Run now**
requests a normal due scan; it does not bypass an authentication hold or retry limits.
**Retry failed** resumes after repair and allows one bounded retry sweep, even if
an authentication hold remains after its failed card left inventory. Current evidence
is not refetched. A request accepted message is not a completed fleet refresh.

Saving Card Ladder credentials activates the shared client and notifies the worker.
If the save succeeds but worker notification fails, the UI says credentials were saved
and directs the administrator to retry activation. Token rotation alone does not
clear an authentication hold. Disabled/unconfigured workers remain visible; cached
selection and packing remain usable during outages.

Inventory rereads stored evaluations on focus, UTC midnight and evidence expiry.
These reads do not collect comps or change selected versions. Unresolved early
observations have a bounded follow-up. Evaluation reads retain a full-response
30-second timeout per attempt and an explicit read retry after failure. Packing
lists use **Update list status** and invalidation after actions instead of these
timers. Old successful sales remain inspectable with their acquisition date.

Use the existing card checkboxes and choose **Add to show** in the same selected-items
bar as Record sale and List on DH. Its **CL value** total is a valuation, not the
DH listed or reviewed price. Choose a saved list or name a new one, then add.
Escape closes destination details before clearing selection. Successful Add links
to the submitted packing list. Creating a list alone does not select inventory.
Unreceived cards can be planned but cannot be packed. Missing or weak comp evidence
does not prohibit a physically eligible manual shortlist.

If selected data changes, rows stay identifiable and Add is blocked until you
explicitly review/reselect; the application never silently acknowledges new prices
or evidence. Normal Inventory retains its operational filters, price bands and
Needs Attention controls; inactive zero-count bands and a quiet attention banner
hide normally, even with a selection. Price review instead provides focused assessment
filters and sorting alongside the persistent price editor. Shared search, selected
cards and unsaved drafts survive switching between Inventory and Price review.
Neither view has the retired price-support/coverage header.
**Reveal selected** recovers cards outside your current view. Financial forms retain
their own pending states and do not wait for comp collection. If an evaluation read
fails, cached facts remain inspectable but do not certify current support. Retry the
read; a successful recovery does not silently advance selected versions.

On the saved list, check **Packed** as you load each slab. Price/support changes
are flagged for review rather than silently acknowledged. A price change after
packing is a reminder to check the physical sticker. Sold, refunded, removed, or
closed-campaign members stay visible with their packing history; they are excluded
from available value and cannot be newly packed. You can still unpack or remove
them explicitly.

Known ready-to-pack asking totals use positive canonical local asking; missing
asking and DH association warnings are counted separately. Historical acknowledged
amounts/status codes and packing times are retained without migration or automatic
acknowledgment. An old `no_listed_price` code meant no DH listed price under the old
policy, not necessarily no local asking. Only explicit Add, Pack or Acknowledge
records the current asking/status. Adding or packing cards **does not** change prices,
delist online inventory, reserve a card, or record a sale. Continue recording sales
through the existing sales workflow. **Update list status** rereads saved prices
and availability; it never requests source acquisition.

Local verification has two real-browser/Go/PostgreSQL modes: explicitly seeded
cached-use preconditions, and actual server-worker population from an empty verified
store before any browser starts. Both stop the worker and block all provider endpoints
for cached operator use, preserving source-request history. A separately counted
background-publication phase verifies held selections and stale Add/Pack conflicts.
A separate seven-case Price review test proves read-only trials and whole-row
immutability, then explicit reviewed-price persistence and nonzero controlled DH
sync/list effects with real services and storage. It also exercises failed-read
recovery, stale Add/Pack and mobile deep links. Reproduction instructions are in
[the real-wire test guide](../web/tests/show-readiness-real.md).
Production access, repricing, backfill, population and rollout still require
separate authorization.

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
