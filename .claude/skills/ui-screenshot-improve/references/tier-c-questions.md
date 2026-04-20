# Tier C — systemic question bank

Tier C covers product-level gaps that no per-page audit surfaces: navigation information architecture, onboarding, missing destinations, cross-page coherence, and system-level surfaces the product should have but doesn't.

Pick **3–5 questions per cycle**, rotating across pillars. Don't answer the same set twice in a row — if you answered Navigation & IA last cycle, pick a different pillar this cycle. Add pillars as the product evolves.

Tier C findings use the structural sentence form: "The product does X; it should do Y; because Z."

## Pillar 1 — Navigation & IA

1. Is the primary nav keyed to user tasks (Intake / Selling / Reviewing / Tuning) or to data models (Inventory / Campaigns / Admin)? If data-model-keyed, is that the right choice for this user?
2. Is there a global search? If not, how does a user find a specific slab by cert or name?
3. Do URLs map to user concepts ("/campaigns/pokemon-base-set") or to internal IDs only ("/campaigns/42")?
4. Can a user tell where they are from the URL alone?

## Pillar 2 — Onboarding & Empty States

5. What does a user with zero campaigns, zero purchases, and zero intake see on the dashboard? Is that screen useful or is it an empty canvas?
6. Is there a first-run flow that teaches the primary loop (intake → campaign → sell)?
7. Do empty states *teach* ("no campaigns yet — here's how to make one") or just describe absence ("No campaigns")?
8. Is there a sample-data mode a new user can turn on to explore without polluting real data?

## Pillar 3 — Missing Destinations

9. Do inline counts/chips/callouts that imply drill-in actually go somewhere? (e.g. "2 unpaid invoices" chip → is there an /invoices page?)
10. Is there a "recent activity" or notifications feed?
11. Is there a single "what changed since I last logged in" surface?
12. Do errors and warnings link to the relevant remediation page or just sit as text?

## Pillar 4 — Product Coherence

13. Do admin pages feel like part of the same product — same type ramp, card shell, spacing rhythm — or do they read as a different app?
14. Do modals feel visually connected to the page that spawned them?
15. Is there *one memorable thing* — a signature element that gives the product a point of view — or is every page rendering data with no design voice?

## Pillar 5 — System-Level Surfaces

16. Is there a single "capital position over time" view? If not, how does a user answer "how is my cash changing"?
17. Is there a "what's selling this week" feed?
18. Is there a cross-campaign "portfolio health" summary that a new user understands in 10 seconds?
19. Is there a notifications or tasks list that aggregates things the user should act on?
20. Is there a way to see the product's activity over a rolling window (7/30/90 days) without assembling it from multiple pages?

## Custom pillars

Add your own as the product evolves. A good pillar names a *class of systemic friction*, not an individual page. If you find yourself writing a pillar that names a specific component, that's probably a Tier A finding in disguise.
