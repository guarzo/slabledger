# Product

## Register

product

## Users

A solo PSA Direct Buy operator (the Card Yeti business) plus a small ring of trusted partners and collaborators. Power users running a real grading-and-flipping operation: they already know what PSA 10, CL%, fill rate, days-to-sell, and sell-through mean. They open the app multiple times a day from a desktop, often with multiple tabs and a spreadsheet open alongside. The job is never "browse" — it's always a specific question: *Is this campaign making money? Which inventory is aging? Did Brady's invoice arrive? What should I list on DH this morning?* Sessions are short, dense, and goal-directed.

## Product Purpose

SlabLedger is the operator's source of truth for PSA Direct Buy campaigns: what got bought, what sold where, what's still sitting, and whether the whole portfolio is making money. It exists because spreadsheets can't keep up with multi-channel sell-through (eBay, TCGPlayer, DH, in-person, local), aging inventory, capital-at-risk, and AI-assisted decisioning across hundreds of cards.

Success looks like: the operator opens the dashboard, reads ROI / Deployed / Recovered / At Risk / Wks-to-Cover in one glance, drills into a campaign or aging bucket in one click, and acts (reprice, deactivate, push to DH, send invoice). No hand-holding, no marketing copy, no friction between question and answer.

## Brand Personality

**Operator-grade, telegraphic, confident.** Three words: *dense, exact, calm.*

- Voice is second-person only at the boundaries (login, empty states). Working surfaces are nounal: `Deployed`, `Recovered`, `At Risk`, `Outstanding`, `30d Recovery`. Numbers carry the page; copy gets out of the way.
- Every word earns its place. No "Welcome back!", no "Looks like you don't have any campaigns yet — let's create one together." Errors are flat and instructional: "Authentication failed. Please try again."
- The aesthetic is the dark-mode terminal cousin of Linear/Stripe — fast, dense, tabular-num everywhere — with one piece of operator personality: the Pokemon-themed grade colors and the occasional gold glow on a PSA 10. Not cute; functional polychrome.

## Anti-references

- **SaaS-cream marketing dashboards.** No hero illustrations, no friendly mascots, no "Let's get started!" copy on working surfaces. The login page is the only marketing moment.
- **Generic admin templates** (Material/Bootstrap admin kits, AdminLTE, Tailwind dashboard starter clones). Every card the same size, identical icon-heading-paragraph cells, no information density.
- **Crypto-bro neon-on-black.** Saturated electric green P&L, gradient text, glowing everything. We use one indigo accent and one gold for PSA 10. The rest is restraint.
- **Consumer card-collector apps** (TCGPlayer's customer UI, Collectr, Ludex). This is a back-office tool for someone running a business; it should look more like Linear than like a hobbyist scrapbook.
- **Side-stripe alert callouts, gradient-text headlines, hero-metric template** (big number + small label + supporting stats + gradient accent — a SaaS cliché). Banned absolutely.

## Design Principles

1. **Numbers carry the page.** Tabular-nums everywhere. ROI, capital state, and aging are typography-first, not card-grid-first. The HeroStatsBar — one oversize ROI on the left, supporting stats in a row, hairline divider — is the signature; resist boxing every metric into a card.
2. **Density without noise.** Use spacing rhythm, not borders, to separate rows. Hairline `rgba(255,255,255,0.03)` row dividers; near-invisible `rgba(255,255,255,0.02)` row hover. The eye should track without lines pulling it.
3. **Functional polychrome, restrained chrome.** Color carries meaning — grade ramp (1→10), channel hues, semantic green/yellow/red for money state, indigo for interaction. Outside those roles, the surface is tinted neutrals. No decorative gradients on whole sections.
4. **Operator confidence, no hand-holding.** Domain vocabulary is exact and unexpanded (`CL%`, `Wks to Cover`, `30d Recovery`). Empty states give a numbered checklist (`Create a campaign`, `Import PSA purchases`, `Record sales as you go`), not a tour.
5. **Glass is rare and deliberate.** Two surfaces only — the sticky header and the login card. Don't stack glass-on-glass. The whole point is the single floating plane.

## Accessibility & Inclusion

- WCAG **AA** target. Semantic palette already tuned for dark-on-bright contrast (`--success #34d399`, `--warning #fbbf24`, `--danger #f87171`, `--info #22d3ee`).
- 3px focus ring offset 3px, with a translucent halo (`--color-focus #60a5fa`, ring `rgba(96,165,250,0.2)`). Visible on every focusable element including indigo-filled buttons.
- `prefers-reduced-motion: reduce` honored explicitly in `base.css` — entries, glow pulses, and the Pokéball loader all collapse.
- Color is never the sole signal. Money-state tokens (`--state-waiting`, `--state-at-risk`, `--state-problem`) pair with text labels (`Outstanding`, `At Risk`, `Below Cost`); P&L rows pair the green/red border-strip with sign on the number. Grade badges combine hue with the literal grade text (`PSA 10`, `BGS 10`).
- Single-operator product, but assume desktop keyboard-driven use: every interactive surface keyboard-reachable, focus order matches visual order, hotkeys preferred over mouse for power-user actions.
