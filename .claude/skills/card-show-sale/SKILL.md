---
name: card-show-sale
description: Record an in-person card-show sale from photos of price-stickered slabs — extract cert number and sticker comp per card, preview against inventory, and confirm with one negotiated percentage applied server-side
---

# Card Show Sale

Records a batch of in-person sales where a dealer bought a stack of slabs at one
negotiated percentage of the sticker comps written on them.

The point of this skill is that the recorded revenue stops being a tautology.
The old bulk-sale path computed `sale_price = clValueCents × pct`, so the
revenue we measured our Card Ladder valuation against was *derived from that same
valuation*. Here the comp comes from the dealer's own sticker, captured per card,
and the negotiated percentage is applied server-side once.

## When to Use

- A card-show or in-person bulk sale where each slab carried a price sticker.
- The user says "record the show sale", "I sold a stack at the show", "here are
  the photos from Saturday", or points at a folder of slab photos.

Do **not** use this for eBay / TCGPlayer / Shopify sales — those have their own
CSV import paths (see the `csv-import` skill).

## Inputs

Ask for anything not supplied:

| Input | Required | Example |
|---|---|---|
| Photo folder (or a typed list — see below) | yes | `~/shows/2026-08-08-nats/` |
| Sale date | yes | `2026-08-08` |
| Negotiated percentage | yes | Ask the user in percent (`72`), send as the fraction `negotiatedPct: 0.72`. The API rejects anything outside `(0, 1]`. |
| Sale channel | yes — always send it explicitly | `local` |
| Total actually received | no, but ask | `4310.00` |

Sale channel has no server-side default: an omitted `saleChannel` previews fine
but fails every item when you confirm (see Troubleshooting). Always send one.

The total received is what makes the preview worth running: the endpoint
reconciles `computedTotalCents` (the sum of `theirCompCents × pct` across
matched cards) against it and reports `reconcileDeltaCents = computedTotalCents
− totalReceivedCents` — positive means the computed total is *higher* than what
you were told was received.

Rounding happens once per card, so it cannot explain much: the server does
`round(theirCompCents × pct)` per item, which drifts at most half a cent per
card — about 10 cents on a 20-card batch, at most $1 on a 200-card batch. A
delta bigger than that is a real discrepancy (a mis-stickered card, a mistyped
comp, a cash adjustment nobody mentioned), not rounding.

If one or more certs landed in `notFound` or `alreadySold`, their value is
excluded from `computedTotalCents` entirely — a single unmatched cert alone
produces a delta about the size of that one card. Resolve those certs first;
only recount the physical stack if the delta persists afterward.

If you never got a total from the user, `reconcileDeltaCents` still comes back
as `0` — that's the field's default, not a clean reconciliation. Report it as
"no total supplied, reconciliation not performed," never as "delta 0."

## Procedure

### 1. Extract

Read each photo with the Read tool — it reads images natively, so there is no
vision API to call, no dependency to add, and no blob to store. Extract three
things per photo:

- the PSA **cert number** from the label
- the **sticker price** in dollars
- the **card identity** off the label — player/set/year/grade, whatever it
  plainly shows. This is what fills the `card` column below, and it's the
  value step 3 compares the server's returned `cardName` against.

Build the list in memory. Nothing is uploaded.

### 2. Review — before any network call

Print the full table and stop. The user reads it against the physical stack.

```
 #  cert         card                              sticker
 1  05442200     2021 Topps Chrome Acuna PSA 10    $ 45
 2  17979513     2020 Prizm Herbert PSA 9          $120
 3  6098919001   2018 Optic Doncic PSA 10          $380
 ---
 3 cards, sticker total $545, at 72% = $392.40
```

Ask for corrections. Apply them. Do not proceed until the user says the table is
right.

### 3. Preview

**Convert dollars to cents before sending: multiply every dollar figure by
100.** `$45` → `4500`. The API takes cents only — it has no dollar mode and
will silently accept `45` as 45 cents, so a missed conversion produces a valid
200 response with every price 100x too low and nothing flags it.

```bash
curl -sX POST http://localhost:8081/api/sales/import-certs \
  -H "Authorization: Bearer $LOCAL_API_TOKEN" \
  -H "Content-Type: application/json" \
  -d '{
    "saleDate": "2026-08-08",
    "saleChannel": "local",
    "negotiatedPct": 0.72,
    "totalReceivedCents": 39240,
    "items": [
      {"certNumber": "05442200", "theirCompCents": 4500},
      {"certNumber": "17979513", "theirCompCents": 12000},
      {"certNumber": "6098919001", "theirCompCents": 38000}
    ]
  }'
```

Report to the user:

- `matched` count, and for each match its `cardName` and `salePriceCents`.
  **Compare each returned `cardName` against what you recorded for that cert in
  the step-2 table.** A mismatch means the cert was misread and matched a
  *different* card you own — this is the primary defense against that failure
  mode (see "Cert numbers are the risk" below). Do not confirm until it's
  resolved.
- `notFound` certs, with their `reason`: `not_found` (cert isn't in inventory —
  usually a misread digit), `invalid_comp` (the sticker was read as `$0` or
  blank — not necessarily a misread; confirm whether the card was genuinely
  thrown in free), or `lookup_error` (a sale lookup failed server-side)
- `alreadySold` certs — a card recorded as sold twice is a real data problem.
  There is no override endpoint for this; check and correct the existing sale
  in the app itself rather than re-running the batch.
- `computedTotalCents` next to the user's stated total, and
  `reconcileDeltaCents` — see the rounding/omission caveats under Inputs above
- **every cert in `nearDuplicateCerts`** — see the warning below

### 4. Confirm — only on explicit approval

Never auto-confirm. Never chain step 3 into step 4 in one turn. The user must
say yes after seeing the preview.

```bash
curl -sX POST http://localhost:8081/api/sales/import-certs/confirm \
  -H "Authorization: Bearer $LOCAL_API_TOKEN" \
  -H "Content-Type: application/json" \
  -d '[
    {"purchaseId": "8321", "saleChannel": "local", "saleDate": "2026-08-08",
     "salePriceCents": 3240, "theirCompCents": 4500, "priceSource": "itemized"},
    {"purchaseId": "8347", "saleChannel": "local", "saleDate": "2026-08-08",
     "salePriceCents": 8640, "theirCompCents": 12000, "priceSource": "itemized"}
  ]'
```

`purchaseId` and `salePriceCents` come from the preview's `matched` entries — do
not recompute the price locally. `priceSource` is always `"itemized"` on this
path; that is the whole reason the path exists.

**Read the response before telling the user the sale is recorded.** It's
`{"created": N, "failed": N, "errors": [{"purchaseId": ..., "error": "..."}]}`.
Confirm is **partial-success**: it can write some sales and reject others while
still returning HTTP 200 — a batch with 2 failures out of 20 looks identical to
a clean run unless you read `failed` and `errors`. Always report `created` and
`failed` to the user. If `failed > 0`, print every `errors[]` entry with its
`purchaseId` and message, and say plainly: those cards were **not** recorded
and remain unsold in inventory. See Troubleshooting for the common error
strings and what each one means.

## Cert numbers are the risk, not prices

A sticker price is 2-4 digits and lives next to a card the user is looking at. A
misread price is obvious by eye at the review table.

Cert numbers are not. They are 8-10 digits with **no checksum** and **variable
width** — real examples: `05442200`, `17979513`, `6098919001`. One wrong digit
does one of two things:

- **fails to match** → lands in `notFound`. This is fine. It is self-flagging.
- **matches a different card you own** → records a sale against the wrong slab,
  marks it sold, and leaves the real card in inventory. The `cardName` check in
  step 3 is the defense against this — the preview returns the *matched* card's
  name, so a misread cert shows up as the wrong card name if you compare it.

`nearDuplicateCerts` in the response helps too, but read its guarantee
precisely: it only compares the certs **you submitted in this batch** against
each other — it never checks against the rest of your inventory. Any cert
listed there is within one digit of *another cert in the same batch*. An empty
`nearDuplicateCerts` does **not** mean no cert here is close to something you
own — it means no two certs *you submitted* are close to each other. A single
misread cert that collides with a card sitting at home, not in this stack, is
not flagged by `nearDuplicateCerts` at all; the `cardName` comparison above is
what catches that case. When a cert *is* listed in `nearDuplicateCerts`,
**re-open that card's photo and re-read the label digit by digit before
confirming.** Do not reason about which is "more likely" — look at the
picture.

(Exact duplicate certs — the same card photographed twice — are deliberately
not flagged by `nearDuplicateCerts`; that's not a misread. The preview returns
two matched rows for the same purchase, and the server catches the actual
duplicate write at confirm time as a `failed` entry, not a double sale — one
more reason to read the confirm response.)

The same reasoning is why one card per photo is non-negotiable (below): a frame
with two slabs is the one failure mode that produces a plausible, unflagged,
wrong pairing.

## Typed-list input mode

Photos are a convenience, not a dependency. If they came out badly — glare,
motion blur, wrong folder — accept a pasted list instead and skip step 1
entirely:

```
05442200,45
17979513,120
6098919001,380
```

One `cert,price` pair per line, price in dollars. Everything from step 2 onward
is identical, with one difference: there's no photo to read a card identity
from, so the review table's `card` column is blank in this mode. That makes
the step-3 `cardName` comparison *more* important here, not less — but the
referent shifts: with no photo captured, compare the server's returned
`cardName` against the physical card in the user's hand, reading the slab
directly rather than against a table entry. A batch is always recordable even
when OCR is useless.

## Capture requirements

Tell the user this *before* the show, not after. These are the conditions under
which step 1 works:

- **Sticker on the front, price written plainly.** `45`. Not `$45.00`, not
  `45/72%`, not `45 → 40`. A sticker encoding the negotiation is a sticker the
  extraction has to guess at.
- **One card per photo.** Two slabs in one frame reintroduces sticker-to-cert
  ambiguity, and it is the only failure mode here that is not self-flagging.
- **Full PSA label in frame, including the barcode.** Crop the card art before
  the label, never the label.
- **Even light.** Slabs are shiny; hard overhead light blows out the label and
  the cert digits go first.

## Troubleshooting

| Symptom | Cause | Fix |
|---|---|---|
| Cert in `notFound`, reason `not_found` | misread digit, or the card was never in inventory | re-read the photo; if the cert is right, the card needs importing first (`csv-import` skill) |
| Cert in `notFound`, reason `invalid_comp` | sticker read as `$0` or blank — not necessarily a misread | confirm whether the card was genuinely thrown in for free; there's nothing to re-read if so |
| Cert in `alreadySold` | double-recorded, or a genuine re-sale after a return | there is no override — check and correct the existing sale in the app itself |
| Cert in `nearDuplicateCerts` | within one digit of another cert *in this same batch* (not checked against the rest of inventory) | re-read both photos digit by digit; do not confirm the batch until resolved |
| One or more certs in `notFound`/`alreadySold`, and the reconcile delta is about one card's size | their value is excluded from `computedTotalCents`, so a single unmatched cert alone produces a delta this size | resolve those certs first; recount the physical stack only if the delta persists afterward |
| Reconcile delta of roughly a dollar or more | rounding alone cannot exceed ~half a cent per card, so this is a real discrepancy: a mis-stickered card, a mistyped comp, or an unrecorded cash adjustment | recount the physical stack against the review table |
| Reconcile delta of a few cents on a normal-size batch | per-card rounding | expected; proceed |
| Reconcile delta is exactly `0` and no `totalReceivedCents` was sent | the field defaults to `0` when no total was supplied — not a real reconciliation | report it as "no total supplied," not as clean |
| Confirm response has `failed > 0` | one or more items were rejected at write time even though the batch returned HTTP 200 | read every `errors[]` entry; those cards are **not** recorded and remain unsold in inventory |
| `errors[]` message `"sale date cannot be before purchase date"` | the typed show date doesn't parse as on/after that card's purchase date | correct the sale date, not the price |
| `errors[]` message `"already sold"` at confirm, despite a clean preview | inventory state changed between preview and confirm, or the same cert appeared twice in the batch | check the existing sale before re-running |
| Omitted `saleChannel` | previews fine (no validation there) but fails every item at confirm | always send an explicit `saleChannel` |
| `401 Unauthorized` | `LOCAL_API_TOKEN` unset or stale | `echo $LOCAL_API_TOKEN`; it must match the running server's config |
