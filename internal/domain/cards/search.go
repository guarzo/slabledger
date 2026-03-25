package cards

import (
	"context"
	"fmt"
	"regexp"
	"sort"
	"strings"
)

// Pre-compiled regex patterns for card search parsing
var (
	cardSearchNumberPattern = regexp.MustCompile(`(?i)#?(\d{1,3}(?:/\d{1,3})?|[A-Z]{1,3}\d{1,3})`)
)

// setKeywords contains common Pokemon TCG set names and abbreviations used for
// search query parsing. These keywords are matched case-insensitively against
// user queries to extract set name filters. Ordered roughly chronologically
// from oldest to newest sets for easier maintenance.
var setKeywords = []string{
	"base set", "base", "jungle", "fossil", "team rocket",
	"neo genesis", "neo", "gym heroes", "gym challenge",
	"legendary collection", "expedition", "aquapolis", "skyridge",
	"ruby sapphire", "sandstorm", "dragon", "team magma vs team aqua",
	"hidden legends", "fire red leaf green", "team rocket returns",
	"deoxys", "emerald", "unseen forces", "delta species",
	"legend maker", "holon phantoms", "crystal guardians",
	"diamond pearl", "mysterious treasures", "secret wonders",
	"great encounters", "majestic dawn", "legends awakened",
	"stormfront", "platinum", "rising rivals", "supreme victors",
	"arceus", "heartgold soulsilver", "unleashed", "undaunted",
	"triumphant", "black white", "emerging powers", "noble victories",
	"next destinies", "dark explorers", "dragons exalted",
	"boundaries crossed", "plasma storm", "plasma freeze",
	"plasma blast", "legendary treasures", "xy", "flashfire",
	"furious fists", "phantom forces", "primal clash",
	"roaring skies", "ancient origins", "breakthrough",
	"breakpoint", "fates collide", "steam siege", "evolutions",
	"sun moon", "guardians rising", "burning shadows",
	"crimson invasion", "ultra prism", "forbidden light",
	"celestial storm", "lost thunder", "team up", "unbroken bonds",
	"unified minds", "cosmic eclipse", "sword shield",
	"rebel clash", "darkness ablaze", "vivid voltage",
	"shining fates", "battle styles", "chilling reign",
	"evolving skies", "fusion strike", "brilliant stars",
	"astral radiance", "lost origin", "silver tempest",
	"crown zenith", "paldea evolved", "obsidian flames",
	"paradox rift", "paldean fates", "temporal forces",
	"twilight masquerade", "shrouded fable", "stellar crown",
	"surging sparks",
}

// parsedQuery represents a parsed search query with extracted components.
type parsedQuery struct {
	CardName   string
	SetName    string
	CardNumber string
	Tokens     []string
}

// parseSearchQuery parses a free-form search query into components.
func parseSearchQuery(query string) parsedQuery {
	query = strings.TrimSpace(query)

	// Extract card number patterns (e.g., "#123", "123/150", "SV001")
	cardNumber := ""
	if match := cardSearchNumberPattern.FindString(query); match != "" {
		cardNumber = strings.TrimPrefix(match, "#")
		query = cardSearchNumberPattern.ReplaceAllString(query, "") // Remove from query
	}

	// Normalize whitespace
	query = strings.Join(strings.Fields(query), " ")

	// Extract set name from query using package-level setKeywords
	setName := ""
	queryLower := strings.ToLower(query)
	for _, keyword := range setKeywords {
		idx := strings.Index(queryLower, keyword)
		if idx < 0 {
			continue
		}
		// Require whole-token boundaries so "neo" doesn't match inside "neon"
		if idx > 0 && isAlphaNum(queryLower[idx-1]) {
			continue
		}
		end := idx + len(keyword)
		if end < len(queryLower) && isAlphaNum(queryLower[end]) {
			continue
		}
		setName = keyword
		query = strings.TrimSpace(query[:idx] + query[end:])
		break
	}

	// Remaining text is card name
	cardName := strings.TrimSpace(query)

	// Split into tokens for matching
	tokens := strings.Fields(strings.ToLower(cardName))

	return parsedQuery{
		CardName:   cardName,
		SetName:    setName,
		CardNumber: cardNumber,
		Tokens:     tokens,
	}
}

// Search relevance scoring weights.
const (
	scoreExactNumber   = 100.0
	scorePartialNumber = 50.0
	scoreExactName     = 80.0
	scorePrefixName    = 60.0
	scoreContainsName  = 40.0
	scoreTokenMatch    = 10.0
	scoreSetMatch      = 20.0
	scoreHasImage      = 5.0
	scoreSecretRare    = 3.0
	scoreRare          = 1.0
)

// scoreCard calculates relevance score for a card against a parsed query.
func scoreCard(card Card, query parsedQuery) float64 {
	score := 0.0

	cardNameLower := strings.ToLower(card.Name)

	// Exact card number match (highest priority)
	if query.CardNumber != "" {
		if strings.EqualFold(card.Number, query.CardNumber) {
			score += scoreExactNumber
		} else if strings.Contains(strings.ToLower(card.Number), strings.ToLower(query.CardNumber)) {
			score += scorePartialNumber
		} else {
			// If searching for specific card number but doesn't match, skip
			return 0
		}
	}

	// Card name matching
	if query.CardName != "" {
		queryNameLower := strings.ToLower(query.CardName)

		if cardNameLower == queryNameLower {
			score += scoreExactName
		} else if strings.HasPrefix(cardNameLower, queryNameLower) {
			score += scorePrefixName
		} else if strings.Contains(cardNameLower, queryNameLower) {
			score += scoreContainsName
		} else {
			// Token-based matching
			matchedTokens := 0
			for _, token := range query.Tokens {
				if len(token) < 2 {
					continue // Skip very short tokens
				}
				if strings.Contains(cardNameLower, token) {
					matchedTokens++
					score += scoreTokenMatch
				}
			}

			// Require at least some token matches
			if len(query.Tokens) > 0 && matchedTokens == 0 {
				return 0
			}
		}
	}

	// Set name matching (bonus points)
	if query.SetName != "" {
		setNameLower := strings.ToLower(card.SetName)
		querySetLower := strings.ToLower(query.SetName)

		if strings.Contains(setNameLower, querySetLower) {
			score += scoreSetMatch
		}
	}

	// Boost for cards with images
	if card.ImageURL != "" {
		score += scoreHasImage
	}

	// Boost for rarer cards
	rarity := strings.ToLower(card.Rarity)
	if strings.Contains(rarity, "secret") || strings.Contains(rarity, "ultra") {
		score += scoreSecretRare
	} else if strings.Contains(rarity, "rare") {
		score += scoreRare
	}

	return score
}

// isAlphaNum reports whether b is an ASCII letter or digit.
func isAlphaNum(b byte) bool {
	return (b >= 'a' && b <= 'z') || (b >= 'A' && b <= 'Z') || (b >= '0' && b <= '9')
}

// SearchService performs card search with relevance scoring.
type SearchService struct {
	cardProv CardProvider
}

// NewSearchService creates a new SearchService.
func NewSearchService(cardProv CardProvider) *SearchService {
	return &SearchService{cardProv: cardProv}
}

// SearchResult represents a ranked card search result.
type SearchResult struct {
	Card  Card
	Score float64
}

// Search parses the query, searches via CardProvider, scores and ranks results.
func (s *SearchService) Search(ctx context.Context, query string, limit int) ([]SearchResult, int, error) {
	// Parse the query to extract potential components
	parsed := parseSearchQuery(query)

	// Build search criteria for the provider
	criteria := SearchCriteria{
		Query:      query,
		CardName:   parsed.CardName,
		SetName:    parsed.SetName,
		CardNumber: parsed.CardNumber,
		Limit:      limit,
	}

	// Check for context cancellation before expensive provider call
	if ctx.Err() != nil {
		return nil, 0, ctx.Err()
	}

	// Use the provider's SearchCards method to avoid O(N) full catalogue scan
	providerCards, totalCount, err := s.cardProv.SearchCards(ctx, criteria)
	if err != nil {
		return nil, 0, fmt.Errorf("card provider unavailable: %w", err)
	}

	// Score and rank results
	var scoredResults []SearchResult
	for _, card := range providerCards {
		if ctx.Err() != nil {
			break
		}

		score := scoreCard(card, parsed)
		if score > 0 {
			scoredResults = append(scoredResults, SearchResult{
				Card:  card,
				Score: score,
			})
		}
	}

	// Sort by relevance score (highest first), with deterministic tiebreaker
	sort.SliceStable(scoredResults, func(i, j int) bool {
		if scoredResults[i].Score != scoredResults[j].Score {
			return scoredResults[i].Score > scoredResults[j].Score
		}
		// Tiebreaker: name, then set, then number
		if scoredResults[i].Card.Name != scoredResults[j].Card.Name {
			return scoredResults[i].Card.Name < scoredResults[j].Card.Name
		}
		if scoredResults[i].Card.SetName != scoredResults[j].Card.SetName {
			return scoredResults[i].Card.SetName < scoredResults[j].Card.SetName
		}
		return scoredResults[i].Card.Number < scoredResults[j].Card.Number
	})

	// Limit results
	if len(scoredResults) > limit {
		scoredResults = scoredResults[:limit]
	}

	return scoredResults, totalCount, nil
}
