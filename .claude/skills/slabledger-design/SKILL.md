---
name: slabledger-design
description: Use this skill to generate well-branded interfaces and assets for SlabLedger, either for production or throwaway prototypes/mocks. Contains essential design guidelines, colors, type, fonts, assets, and UI kit components for prototyping a PSA-card portfolio/ledger tool.
user-invocable: true
---

Read the README.md file within this skill, and explore the other available files.

This skill covers **SlabLedger** — an internal dark-mode web tool for managing PSA-graded Pokemon card Direct Buy campaigns (tracking, inventory, P&L, sales across channels). Related operator brand: **Card Yeti**.

Key files:
- `README.md` — product context, content + visual foundations, iconography
- `colors_and_type.css` — all CSS custom properties (copy into your HTML's `<link>` or inline)
- `assets/` — logos (`slabledger-card-logo.png`, `card-yeti-logo.png`, `favicon.ico`)
- `ui_kits/slabledger-web/` — React components: Button, GradeBadge, StatusPill, RecommendationBadge, HeroStatsBar, LedgerTable, Header, LoginCard, plus full page recreations
- `preview/` — small HTML specimens of each token/component

If creating visual artifacts (slides, mocks, throwaway prototypes, etc), copy assets out and create static HTML files for the user to view. Use the `ui_kits/slabledger-web/*.jsx` components as starting points — they're already wired to the design tokens.

If working on production code, read the rules in `README.md` (especially the Content Fundamentals and Visual Foundations sections) to become an expert in designing with this brand. Honor the dark-mode-only, system-font, tabular-num, inline-SVG-icon conventions.

If the user invokes this skill without any other guidance, ask them what they want to build or design, ask some focused questions (which surface? dashboard, campaign form, login, report?), and act as an expert designer who outputs HTML artifacts or production code, depending on the need.
