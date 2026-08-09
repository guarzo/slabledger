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
| Sale channel | no, defaults to `local` | `local` |
| Total actually received | no, but ask | `4310.00` |

The total received is what makes the preview worth running: the endpoint
reconciles `sum(theirCompCents × pct)` against it and reports the delta. A delta
of a few dollars is rounding. A delta the size of one card means a card is
missing from the batch or a sticker was misread.

## Procedure

### 1. Extract

Read each photo with the Read tool — it reads images natively, so there is no
vision API to call, no dependency to add, and no blob to store. Extract two
things per photo:

- the PSA **cert number** from the label
- the **sticker price** in dollars

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

- `matched` count, and the per-card sale price the server computed
  (`salePriceCents` on each match)
- `notFound` certs — these are the safe failure; a misread digit usually lands here
- `alreadySold` certs — a card recorded as sold twice is a real data problem
- the reconcile delta (`reconcileDeltaCents`, computed against your
  `totalReceivedCents`)
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

## Cert numbers are the risk, not prices

A sticker price is 2-4 digits and lives next to a card the user is looking at. A
misread price is obvious by eye at the review table.

Cert numbers are not. They are 8-10 digits with **no checksum** and **variable
width** — real examples: `05442200`, `17979513`, `6098919001`. One wrong digit
does one of two things:

- **fails to match** → lands in `notFound`. This is fine. It is self-flagging.
- **matches a different card you own** → records a sale against the wrong slab,
  marks it sold, and leaves the real card in inventory. Nothing flags it.

That second case is why `nearDuplicateCerts` exists in the response. Any cert
listed there is within one digit of another owned cert. **Re-open that card's
photo and re-read the label digit by digit before confirming.** Do not reason
about which is "more likely" — look at the picture.

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
is identical. A batch is always recordable even when OCR is useless.

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
| Cert in `notFound` | misread digit, or the card was never in inventory | re-read the photo; if the cert is right, the card needs importing first (`csv-import` skill) |
| Cert in `alreadySold` | double-recorded, or a genuine re-sale after a return | check the existing sale before overriding |
| Cert in `nearDuplicateCerts` | within one digit of another owned cert | re-read that card's photo digit by digit; do not confirm the batch until resolved |
| Reconcile delta ≈ one card's value | a card is missing from the batch, or one sticker was misread | recount the physical stack against the review table |
| Reconcile delta of a few dollars | per-card rounding | expected; proceed |
| `401 Unauthorized` | `LOCAL_API_TOKEN` unset or stale | `echo $LOCAL_API_TOKEN`; it must match the running server's config |
