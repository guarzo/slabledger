package pricecharting

import (
	"strings"
	"sync"
	"time"
)

// UPCDatabase manages in-memory UPC to product ID mappings for Pokemon cards.
type UPCDatabase struct {
	mappings map[string]*UPCMapping
	mu       sync.RWMutex
}

// UPCMapping represents a UPC to product mapping
type UPCMapping struct {
	UPC         string    `json:"upc"`
	ProductID   string    `json:"product_id"`
	ProductName string    `json:"product_name"`
	SetName     string    `json:"set_name"`
	CardNumber  string    `json:"card_number"`
	Variant     string    `json:"variant,omitempty"`  // "1st Edition", "Shadowless", etc.
	Language    string    `json:"language,omitempty"` // "English", "Japanese", etc.
	LastUpdated time.Time `json:"last_updated"`
	Confidence  float64   `json:"confidence"` // 0.0 to 1.0
}

// NewUPCDatabase creates a new in-memory UPC database
func NewUPCDatabase() *UPCDatabase {
	return &UPCDatabase{
		mappings: make(map[string]*UPCMapping),
	}
}

// Add stores a new UPC mapping
func (db *UPCDatabase) Add(mapping *UPCMapping) {
	db.mu.Lock()
	defer db.mu.Unlock()

	mapping.LastUpdated = time.Now()
	db.mappings[mapping.UPC] = mapping
}

// AddBatch stores multiple UPC mappings
func (db *UPCDatabase) AddBatch(mappings []*UPCMapping) {
	db.mu.Lock()
	defer db.mu.Unlock()

	now := time.Now()
	for _, mapping := range mappings {
		mapping.LastUpdated = now
		db.mappings[mapping.UPC] = mapping
	}
}

// FindByCardInfo searches for UPCs by card details
func (db *UPCDatabase) FindByCardInfo(setName, cardNumber string) []*UPCMapping {
	db.mu.RLock()
	defer db.mu.RUnlock()

	var matches []*UPCMapping
	setLower := strings.ToLower(setName)

	for _, mapping := range db.mappings {
		if strings.ToLower(mapping.SetName) == setLower &&
			mapping.CardNumber == cardNumber {
			matches = append(matches, mapping)
		}
	}
	return matches
}

// PopulateCommonMappings adds common Pokemon TCG UPC mappings
func (db *UPCDatabase) PopulateCommonMappings() {
	// This would be populated from a data source or manually maintained
	// Examples of common Pokemon TCG product UPCs
	commonMappings := []*UPCMapping{
		// Surging Sparks examples
		{
			UPC:         "820650558726",
			ProductID:   "surging-sparks-250",
			ProductName: "Pikachu ex",
			SetName:     "Surging Sparks",
			CardNumber:  "250",
			Variant:     "",
			Language:    "English",
			Confidence:  1.0,
		},
		{
			UPC:         "820650558733",
			ProductID:   "surging-sparks-251",
			ProductName: "Alolan Exeggutor ex",
			SetName:     "Surging Sparks",
			CardNumber:  "251",
			Variant:     "",
			Language:    "English",
			Confidence:  1.0,
		},
		// Prismatic Evolutions examples
		{
			UPC:         "820650559876",
			ProductID:   "prismatic-evolutions-001",
			ProductName: "Eevee",
			SetName:     "Prismatic Evolutions",
			CardNumber:  "001",
			Variant:     "",
			Language:    "English",
			Confidence:  1.0,
		},
		// Japanese examples
		{
			UPC:         "4521329385426",
			ProductID:   "vmax-climax-003",
			ProductName: "Charizard VMAX",
			SetName:     "VMAX Climax",
			CardNumber:  "003",
			Variant:     "",
			Language:    "Japanese",
			Confidence:  1.0,
		},
		// 1st Edition Base Set examples
		{
			UPC:         "0074427891234",
			ProductID:   "base-set-004",
			ProductName: "Charizard",
			SetName:     "Base Set",
			CardNumber:  "004",
			Variant:     "1st Edition",
			Language:    "English",
			Confidence:  1.0,
		},
		{
			UPC:         "0074427891241",
			ProductID:   "base-set-004-shadowless",
			ProductName: "Charizard",
			SetName:     "Base Set",
			CardNumber:  "004",
			Variant:     "Shadowless",
			Language:    "English",
			Confidence:  1.0,
		},
	}

	db.AddBatch(commonMappings)
}
