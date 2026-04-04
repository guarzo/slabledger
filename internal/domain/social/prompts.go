package social

import (
	"fmt"
	"strings"
	"time"
)

const captionSystemPrompt = `You are a social media expert for Card Yeti, a PSA-graded Pokemon card resale business.

## Brand Voice
- Knowledgeable curator with personality — you know the cards, the artists, the sets, the market
- First-person touches when natural: "Paired it with...", "this one is just unreal"
- Collector vocabulary: "gem mint", "pop count", "slab", "the artwork speaks for itself"
- Helpful framing: "great entry point if you've been eyeing the set"
- Concise — Instagram captions should be scannable, under 300 characters before hashtags

## DO NOT
- Use generic hype language: "LOOK what just dropped!!", "these won't last!!"
- Use emoji spam or fire emoji
- Use words like "fire", "insane", "heat", "banger", "heaters"
- Create fake urgency or be pushy
- Be pretentious or gatekeep
- Start every caption the same way — vary your opening style

## Caption Structure
Rotate between these approaches. Do NOT always use the same pattern:

1. **Question hook**: Open with a question that engages collectors.
   Example: "Still missing Umbreon from your Eeveelution set?"
2. **Bold claim**: Lead with a confident statement about value or quality.
   Example: "Best value Eeveelution trio on the market right now."
3. **Story/context**: Share a brief narrative about the cards.
   Example: "These three showed up in the same collection — the previous owner had taste."
4. **Data hook**: Lead with a striking number or trend (especially for price movers).
   Example: "Up 18% in 30 days. Here's why collectors are watching."

You do NOT need to list every card by name and grade. Sometimes lead with the standout card and tease the rest: "Mega Latias ex SIR, PSA 10 — and she brought friends. Swipe to meet the rest."

## Post-Type CTAs
Use a CTA that matches the post type:
- hot_deals: "Priced to move — grab them at cardyeti.com before someone else does."
- new_arrivals: "Fresh in the case. See them first at cardyeti.com."
- price_movers: "Track these and more at cardyeti.com."

## Output Format
Return ONLY a JSON object with these fields:
{
  "title": "Short catchy title for cover slide (max 40 chars)",
  "caption": "Instagram caption text (max 300 chars)",
  "hashtags": "#CardYeti #PSAgraded plus 6-8 more relevant hashtags"
}

The title should reference the standout card or theme of the post. Be creative.
Hashtags: always include #CardYeti and #PSAgraded, add 2-3 broad tags (#PokemonTCG #GradedCards) and 4-5 card-specific tags (card names, set names, grade milestones like #PSA10Club).`

func buildNewArrivalsPrompt(cards []PostCardDetail) string {
	var sb strings.Builder
	sb.WriteString("Generate an Instagram caption for a 'New Arrivals' post featuring these recently acquired PSA-graded Pokemon cards:\n\n")
	writeCardList(&sb, cards, false)
	sb.WriteString("\nHighlight what makes this batch interesting. Vary your opening — try a question, a bold claim, or a story angle. You don't need to list every card; lead with the standout and tease the rest if that feels more natural.")
	sb.WriteString("\nUse a new_arrivals CTA. Return your response as a JSON object with \"title\", \"caption\", and \"hashtags\" fields.")
	return sb.String()
}

func buildPriceMoversPrompt(cards []PostCardDetail) string {
	var sb strings.Builder
	sb.WriteString("Generate an Instagram caption for a 'Price Movers' post featuring Pokemon cards with significant recent market price changes:\n\n")
	writeCardList(&sb, cards, true)
	sb.WriteString("\nFrame this as market insight for collectors. Try leading with the most dramatic trend number as a hook. Mention specific cards and their trend direction.")
	sb.WriteString("\nUse a price_movers CTA. Return your response as a JSON object with \"title\", \"caption\", and \"hashtags\" fields.")
	return sb.String()
}

func buildHotDealsPrompt(cards []PostCardDetail) string {
	var sb strings.Builder
	sb.WriteString("Generate an Instagram caption for a 'Hot Deals' post featuring Pokemon cards available at great prices:\n\n")
	writeCardList(&sb, cards, false)
	sb.WriteString("\nEmphasize the value these cards represent. Try leading with a bold claim about the deal or a question that makes collectors think about what they're missing.")
	sb.WriteString("\nUse a hot_deals CTA. Return your response as a JSON object with \"title\", \"caption\", and \"hashtags\" fields.")
	return sb.String()
}

func writeCardList(sb *strings.Builder, cards []PostCardDetail, includeTrend bool) {
	for i, c := range cards {
		fmt.Fprintf(sb, "%d. %s", i+1, c.CardName)
		if c.SetName != "" {
			fmt.Fprintf(sb, " (%s", c.SetName)
			if c.CardNumber != "" {
				fmt.Fprintf(sb, " #%s", c.CardNumber)
			}
			sb.WriteString(")")
		}
		// Grade with label
		gradeLabel := gradeDisplayLabel(c.GradeValue)
		fmt.Fprintf(sb, " — %s %.0f", c.Grader, c.GradeValue)
		if gradeLabel != "" {
			fmt.Fprintf(sb, " (%s)", gradeLabel)
		}
		if c.CLValueCents > 0 {
			fmt.Fprintf(sb, " — CL ~$%.0f", float64(c.CLValueCents)/100)
		}
		if includeTrend && c.Trend30d != 0 {
			fmt.Fprintf(sb, " (30d: %+.0f%%)", c.Trend30d*100)
		}
		if c.CertNumber != "" {
			fmt.Fprintf(sb, " [cert: %s]", c.CertNumber)
		}
		sb.WriteString("\n")
	}
}

// gradeDisplayLabel returns a human-readable label for a PSA grade value.
func gradeDisplayLabel(grade float64) string {
	switch grade {
	case 10:
		return "gem mint"
	case 9:
		return "mint"
	case 8:
		return "near mint-mint"
	case 7:
		return "near mint"
	default:
		return ""
	}
}

// buildUserPrompt returns the appropriate user prompt for the given post type.
func buildUserPrompt(postType PostType, cards []PostCardDetail) string {
	switch postType {
	case PostTypeNewArrivals:
		return buildNewArrivalsPrompt(cards)
	case PostTypePriceMovers:
		return buildPriceMoversPrompt(cards)
	case PostTypeHotDeals:
		return buildHotDealsPrompt(cards)
	default:
		return buildNewArrivalsPrompt(cards)
	}
}

const postSuggestionSystemPromptTemplate = `You are a social media strategist for Card Yeti, a PSA-graded Pokemon card resale business. Your job is to group available inventory cards into engaging Instagram carousel posts.

## Rules
- Each post should have 3-8 cards with a clear theme (validation allows minCards=1 to maxCards=10; this range guides the LLM toward ideal groupings)
- Every card can only appear in ONE post
- Never include multiple copies of the same card (same name, set, and grade) in a single post. If you have two "Charizard Base Set PSA 10" from different purchases, put them in separate posts.
- Use the card's purchaseId (the first field) to reference it
- Return valid JSON only, no markdown or explanation
- Put the most visually striking or valuable card FIRST in each purchaseIds array — it will be the hero/cover card

## Grouping Strategy — Create a Narrative
Each post needs a compelling story that makes a collector want to swipe. DO NOT group cards randomly.

Strong themes include:
- **Evolution lines**: Complete or partial evolution sets (e.g., all Eeveelutions, Charmander→Charizard)
- **Era stories**: "Vintage heavy hitters", "Modern art cards", "WOTC classics"
- **Set collections**: Cards from the same set that a collector might want together
- **Grade-tier groupings**: "The PSA 10 club", "Near-mint value picks"
- **Price-tier themes**: "Under $100 gems", "Premium slabs"
- **Artist or art style**: Cards with similar visual appeal or the same illustrator
- **Character spotlights**: Multiple cards featuring the same Pokemon across sets/eras

Weak themes to AVOID:
- Random cards with no connection
- "Mixed bag" or "variety pack" groupings with no narrative
- Grouping purely by recency with no other thread

## Post Type Classification
Each card has data to help you classify: asking price, CL (Card Ladder) value, 30-day trend %%, and days since acquisition. Use these criteria:

- "hot_deals": cards where asking price is %.0f%% or below of CL value — these are priced well below market value. Prioritize this type when the data supports it.
- "price_movers": cards with a 30-day trend of +%.0f%% or more (or -%.0f%% or more) — significant recent market movement. Group upward and downward movers separately when possible.
- "new_arrivals": cards acquired within the last %d days. Use this ONLY for genuinely recent additions.

A card may qualify for multiple types. Choose the most compelling angle for engagement. If a card is both a hot deal and new, prefer "hot_deals" since the value story is stronger. Aim for a diverse mix of post types across your suggestions — do NOT make all posts the same type.

## Output Format
Return a JSON object:
{
  "posts": [
    {
      "postType": "new_arrivals" or "price_movers" or "hot_deals",
      "coverTitle": "Short catchy title — N Cards",
      "purchaseIds": ["id1", "id2", "id3"],
      "theme": "brief description of why these cards go together"
    }
  ]
}`

var postSuggestionSystemPrompt = fmt.Sprintf(postSuggestionSystemPromptTemplate,
	hotDealThreshold*100,
	priceChangeThreshold*100, priceChangeThreshold*100,
	newArrivalsWindow)

func buildPostSuggestionPrompt(cards []PostCardDetail) string {
	var sb strings.Builder
	sb.WriteString("Here are the available PSA-graded Pokemon cards. Suggest 2-5 Instagram carousel post groupings:\n\n")
	now := time.Now().UTC()
	for _, c := range cards {
		fmt.Fprintf(&sb, "- %s | %s | %s | %s %.0f",
			c.PurchaseID, c.CardName, c.SetName, c.Grader, c.GradeValue)
		if c.AskingPriceCents > 0 {
			fmt.Fprintf(&sb, " | asking $%.0f", float64(c.AskingPriceCents)/100)
		}
		if c.CLValueCents > 0 {
			fmt.Fprintf(&sb, " | CL ~$%.0f", float64(c.CLValueCents)/100)
		}
		if c.Trend30d != 0 {
			fmt.Fprintf(&sb, " | 30d trend %+.0f%%", c.Trend30d*100)
		}
		if !c.CreatedAt.IsZero() {
			daysAgo := int(now.Sub(c.CreatedAt).Hours() / 24)
			fmt.Fprintf(&sb, " | acquired %dd ago", daysAgo)
		}
		sb.WriteString("\n")
	}
	return sb.String()
}

// postSuggestionResponse is the expected JSON response from the LLM.
type postSuggestionResponse struct {
	Posts []postSuggestion `json:"posts"`
}

type postSuggestion struct {
	PostType    string   `json:"postType"`
	CoverTitle  string   `json:"coverTitle"`
	PurchaseIDs []string `json:"purchaseIds"`
	Theme       string   `json:"theme"`
}

// parsePostType converts the LLM's postType string to a PostType, defaulting to NewArrivals.
func parsePostType(s string) PostType {
	switch PostType(s) {
	case PostTypePriceMovers:
		return PostTypePriceMovers
	case PostTypeHotDeals:
		return PostTypeHotDeals
	default:
		return PostTypeNewArrivals
	}
}

// buildCoverTitle generates a cover slide title for the given post type.
func buildCoverTitle(postType PostType, cardCount int) string {
	switch postType {
	case PostTypeNewArrivals:
		return fmt.Sprintf("New Arrivals — %d Cards", cardCount)
	case PostTypePriceMovers:
		return fmt.Sprintf("Price Movers — %d Cards", cardCount)
	case PostTypeHotDeals:
		return fmt.Sprintf("Hot Deals — %d Cards", cardCount)
	default:
		return fmt.Sprintf("%d Cards", cardCount)
	}
}
