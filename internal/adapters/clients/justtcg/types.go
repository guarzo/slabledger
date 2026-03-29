// Package justtcg provides a client for the JustTCG raw NM pricing API.
//
// Cards have variants (condition + printing combinations), each with a price.
// NM pricing is used for raw card acquisition arbitrage decisions.
package justtcg

// Card represents a TCG card returned from JustTCG search or batch lookup.
type Card struct {
	CardID   string    `json:"cardId"`
	Name     string    `json:"name"`
	SetID    string    `json:"setId"`
	SetName  string    `json:"setName"`
	Number   string    `json:"number"`
	Rarity   string    `json:"rarity"`
	Image    string    `json:"image"`
	Variants []Variant `json:"variants"`
}

// Variant represents a condition + printing combination with its price.
type Variant struct {
	Condition string  `json:"condition"`
	Printing  string  `json:"printing"`
	Price     float64 `json:"price"`
}

// Set represents a TCG set returned from JustTCG set search.
type Set struct {
	ID        string `json:"id"`
	Name      string `json:"name"`
	CardCount int    `json:"cardCount"`
}

// cardsResponse is the API response wrapper for card endpoints.
type cardsResponse struct {
	Data []Card       `json:"data"`
	Meta responseMeta `json:"meta"`
}

// setsResponse is the API response wrapper for set endpoints.
type setsResponse struct {
	Data []Set        `json:"data"`
	Meta responseMeta `json:"meta"`
}

// batchLookupItem is a single item in a batch lookup request body.
type batchLookupItem struct {
	CardID string `json:"cardId"`
}

// responseMeta contains pagination and result metadata from API responses.
type responseMeta struct {
	Total    int `json:"total"`
	Page     int `json:"page"`
	PageSize int `json:"pageSize"`
}

// NMPrice returns the price for a Near Mint card with the given printing.
// Returns 0 if no matching NM variant is found.
func (c Card) NMPrice(printing string) float64 {
	for _, v := range c.Variants {
		if v.Condition == "NM" && v.Printing == printing {
			return v.Price
		}
	}
	return 0
}

// BestNMPrice returns the highest NM price across all printings.
// Returns 0 if no NM variants exist.
func (c Card) BestNMPrice() float64 {
	var best float64
	for _, v := range c.Variants {
		if v.Condition == "NM" && v.Price > best {
			best = v.Price
		}
	}
	return best
}
