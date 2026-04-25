# SlabLedger Web — UI Kit

High-fidelity recreation of the SlabLedger dark dashboard, lifted from `guarzo/slabledger` (`web/src/react`).

Open `index.html` for the click-through prototype: landing on **Login** → Google sign-in → **Dashboard** → navigate to **Campaigns** / **Inventory** via the top nav.

## Components

- `AppChrome.jsx` — sticky glass `Header` (brand mark, live PSA-10 ticker, nav, user menu) + page shell
- `Navigation.jsx` — horizontal primary nav with active-tab indicator
- `Button.jsx` — primary / secondary / success / danger / ghost, sm/md/lg, pressed ripple
- `CardShell.jsx` — default / elevated / glass / premium
- `GradeBadge.jsx` — PSA / BGS / CGC capsules with the functional grade palette
- `StatusPill.jsx` — success/warning/danger/info capsules
- `RecommendationBadge.jsx` — MUST BUY → AVOID, gradient + glow
- `HeroStatsBar.jsx` — the signature dashboard hero row
- `LedgerTable.jsx` — dense tabular-num table with P&L color strips
- `EmptyState.jsx`
- `LoginCard.jsx` — glass card, brand logo, Google sign-in, feature row with emoji
- `pages/*` — `LoginPage`, `DashboardPage`, `CampaignsPage`, `InventoryPage`

Visuals are faithful to the codebase; handlers are stubbed (fake data, in-memory state).
