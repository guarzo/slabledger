package campaigns

import (
	"fmt"
	"regexp"
	"strconv"
	"strings"
	"time"
)

// ImportError captures a per-row import failure.
type ImportError struct {
	Row   int    `json:"row"`
	Error string `json:"error"`
}

// QuickAddRequest contains the minimal data needed to create a purchase from a cert number.
type QuickAddRequest struct {
	CertNumber   string `json:"certNumber"`
	BuyCostCents int    `json:"buyCostCents"`
	CLValueCents int    `json:"clValueCents,omitempty"`
	PurchaseDate string `json:"purchaseDate,omitempty"` // defaults to today
}

// CLExportRow represents a single row parsed from a Card Ladder export CSV.
type CLExportRow struct {
	DatePurchased string  // Already converted to YYYY-MM-DD
	Card          string  // Full card description
	Player        string  // Clean card name (e.g. "Mewtwo-Rev.foil", "Umbreon Ex")
	Set           string  // Set name
	Number        string  // Card number within set
	Condition     string  // e.g. "PSA 8"
	Investment    float64 // Buy cost in dollars
	CurrentValue  float64 // CL current value in dollars
	SlabSerial    string  // Cert number
	Population    int     // Population count
}

// ParseCLDate converts Card Ladder date format (M/D/YYYY) to YYYY-MM-DD.
func ParseCLDate(dateStr string) (string, error) {
	t, err := time.ParseInLocation("1/2/2006", strings.TrimSpace(dateStr), time.UTC)
	if err != nil {
		return "", fmt.Errorf("invalid date %q: expected M/D/YYYY", dateStr)
	}
	return t.Format("2006-01-02"), nil
}

// CLExportEntry represents a row in Card Ladder import CSV format.
type CLExportEntry struct {
	DatePurchased  string  // M/D/YYYY for CL
	CertNumber     string  // Slab serial / cert number
	Grader         string  // Always "PSA" for us
	Investment     float64 // Buy cost in dollars
	EstimatedValue float64 // CL value in dollars
}

// CLRefreshItemResult contains per-row outcome for CL refreshes.
type CLRefreshItemResult struct {
	CertNumber     string `json:"certNumber"`
	CardName       string `json:"cardName,omitempty"`
	OldValueCents  int    `json:"oldValueCents"`
	NewValueCents  int    `json:"newValueCents"`
	Status         string `json:"status"` // "updated", "skipped", "failed"
	Error          string `json:"error,omitempty"`
	SnapshotQueued bool   `json:"snapshotQueued,omitempty"`
}

// GlobalCLRefreshResult summarizes the outcome of a global (cross-campaign) CL value refresh.
type GlobalCLRefreshResult struct {
	Updated    int                               `json:"updated"`
	NotFound   int                               `json:"notFound"`
	Failed     int                               `json:"failed"`
	Errors     []ImportError                     `json:"errors,omitempty"`
	Results    []CLRefreshItemResult             `json:"results,omitempty"`
	ByCampaign map[string]CampaignRefreshSummary `json:"byCampaign,omitempty"`
}

// CampaignRefreshSummary counts updates within a single campaign during a global refresh.
type CampaignRefreshSummary struct {
	CampaignName string `json:"campaignName"`
	Updated      int    `json:"updated"`
}

// GlobalImportResult summarizes the outcome of a global CL import with auto-allocation.
type GlobalImportResult struct {
	Allocated  int                              `json:"allocated"`
	Refreshed  int                              `json:"refreshed"`
	Unmatched  int                              `json:"unmatched"`
	Ambiguous  int                              `json:"ambiguous"`
	Skipped    int                              `json:"skipped"`
	Failed     int                              `json:"failed"`
	Errors     []ImportError                    `json:"errors,omitempty"`
	Results    []GlobalImportItemResult         `json:"results,omitempty"`
	ByCampaign map[string]CampaignImportSummary `json:"byCampaign,omitempty"`
}

// GlobalImportItemResult contains per-row outcome for global imports.
type GlobalImportItemResult struct {
	CertNumber   string   `json:"certNumber"`
	CardName     string   `json:"cardName,omitempty"`
	Grade        float64  `json:"grade,omitempty"`
	Status       string   `json:"status"` // "allocated", "refreshed", "unmatched", "ambiguous", "skipped", "failed"
	CampaignID   string   `json:"campaignId,omitempty"`
	CampaignName string   `json:"campaignName,omitempty"`
	Candidates   []string `json:"candidates,omitempty"`
	Error        string   `json:"error,omitempty"`
	BuyCostCents int      `json:"buyCostCents,omitempty"`
	CLValueCents int      `json:"clValueCents,omitempty"`
	PurchaseDate string   `json:"purchaseDate,omitempty"`
	SetName      string   `json:"setName,omitempty"`
	CardNumber   string   `json:"cardNumber,omitempty"`
	Population   int      `json:"population,omitempty"`
}

// CampaignImportSummary counts allocations and refreshes within a single campaign during a global import.
type CampaignImportSummary struct {
	CampaignName string `json:"campaignName"`
	Allocated    int    `json:"allocated"`
	Refreshed    int    `json:"refreshed"`
}

// PSAExportRow represents a single row parsed from a PSA communication spreadsheet.
type PSAExportRow struct {
	Date           string  // Purchase date (YYYY-MM-DD after conversion)
	Category       string  // Sport/set category
	CertNumber     string  // PSA cert number
	ListingTitle   string  // Full card listing title
	Grade          float64 // PSA grade (1-10, supports half-grades like 8.5)
	PricePaid      float64 // Price paid in dollars
	PurchaseSource string  // Where the card was sourced
	VaultStatus    string  // PSA vault status
	InvoiceDate    string  // Invoice date (YYYY-MM-DD after conversion)
	WasRefunded    bool    // Whether the purchase was refunded
	FrontImageURL  string  // Front card image URL
	BackImageURL   string  // Back card image URL
}

// PSAImportResult summarizes the outcome of a PSA CSV import.
type PSAImportResult struct {
	Allocated             int                              `json:"allocated"`
	Updated               int                              `json:"updated"`
	Refunded              int                              `json:"refunded"`
	Unmatched             int                              `json:"unmatched"`
	Ambiguous             int                              `json:"ambiguous"`
	Skipped               int                              `json:"skipped"`
	Failed                int                              `json:"failed"`
	InvoicesCreated       int                              `json:"invoicesCreated,omitempty"`
	InvoicesUpdated       int                              `json:"invoicesUpdated,omitempty"`
	CertEnrichmentPending int                              `json:"certEnrichmentPending,omitempty"`
	Errors                []ImportError                    `json:"errors,omitempty"`
	Results               []PSAImportItemResult            `json:"results,omitempty"`
	ByCampaign            map[string]CampaignImportSummary `json:"byCampaign,omitempty"`
}

// PSAImportItemResult contains per-row outcome for PSA imports.
type PSAImportItemResult struct {
	CertNumber   string   `json:"certNumber"`
	CardName     string   `json:"cardName,omitempty"`
	Grade        float64  `json:"grade,omitempty"`
	Status       string   `json:"status"` // "allocated", "updated", "refunded", "unmatched", "ambiguous", "skipped", "failed"
	CampaignID   string   `json:"campaignId,omitempty"`
	CampaignName string   `json:"campaignName,omitempty"`
	Candidates   []string `json:"candidates,omitempty"`
	Error        string   `json:"error,omitempty"`
	BuyCostCents int      `json:"buyCostCents,omitempty"`
	PurchaseDate string   `json:"purchaseDate,omitempty"`
	SetName      string   `json:"setName,omitempty"`
	CardNumber   string   `json:"cardNumber,omitempty"`
}

// PSACardMetadata bundles card identity fields parsed from a PSA listing title.
// Passed through the import pipeline to avoid multi-return and many-parameter functions.
type PSACardMetadata struct {
	CardName     string
	CardNumber   string
	SetName      string
	CardYear     int
	ParseWarning string // non-empty when fallback parsing was used
}

// ParsePSADate converts PSA date formats to YYYY-MM-DD.
// PSA uses formats like "M/D/YYYY" or "YYYY-MM-DD".
func ParsePSADate(dateStr string) (string, error) {
	dateStr = strings.TrimSpace(dateStr)
	if dateStr == "" {
		return "", nil
	}
	// Try ISO format first
	if _, err := time.Parse("2006-01-02", dateStr); err == nil {
		return dateStr, nil
	}
	// Try M/D/YYYY
	t, err := time.ParseInLocation("1/2/2006", dateStr, time.UTC)
	if err != nil {
		return "", fmt.Errorf("invalid date %q: expected M/D/YYYY or YYYY-MM-DD", dateStr)
	}
	return t.Format("2006-01-02"), nil
}

// CLCardName returns the best available card name from CL export data.
// Prefers the Player field (clean name like "Umbreon Ex") over
// the Card field (verbose like "2025 Pokemon ... Umbreon Ex ... PSA 10").
func CLCardName(row CLExportRow) string {
	if row.Player != "" {
		return row.Player
	}
	return row.Card
}

// ShopifyExportRow represents a single row parsed from a Shopify product export CSV.
type ShopifyExportRow struct {
	Handle        string  // Product handle (groups variants)
	CertNumber    string  // PSA cert number (from "cert number" column or SKU PSA-XXXXX pattern)
	Title         string  // Product title (empty for variant-only rows)
	CardName      string  // Extracted card name
	CardNumber    string  // Extracted card number
	SetName       string  // Extracted set name
	Grader        string  // e.g. "PSA", "CGC", "BGS", "SGC"
	GradeValue    float64 // Numeric grade value
	VariantPrice  float64 // Variant price in dollars
	CostPerItem   float64 // Cost per item in dollars
	FrontImageURL string  // Product image URL
	BackImageURL  string  // Additional image URL
}

// ExternalImportResult summarizes the outcome of an external CSV import.
type ExternalImportResult struct {
	Imported int                        `json:"imported"`
	Skipped  int                        `json:"skipped"`
	Updated  int                        `json:"updated"`
	Failed   int                        `json:"failed"`
	Errors   []ImportError              `json:"errors,omitempty"`
	Results  []ExternalImportItemResult `json:"results,omitempty"`
}

// ExternalImportItemResult contains per-row outcome for external imports.
type ExternalImportItemResult struct {
	CertNumber string `json:"certNumber"`
	CardName   string `json:"cardName,omitempty"`
	SetName    string `json:"setName,omitempty"`
	CardNumber string `json:"cardNumber,omitempty"`
	Status     string `json:"status"` // "imported", "skipped", "updated", "failed"
	Error      string `json:"error,omitempty"`
}

// titleCleanupRegex strips grader/grade, condition, and seller note patterns from a product title.
var titleCleanupRegex = regexp.MustCompile(`(?i)\b(CGC|BGS|PSA|SGC)\s*\d{1,2}(?:\.\d)?\b`)

// conditionSuffixRegex matches trailing condition/note patterns like "- Near Mint", "NM", "LP", etc.
var conditionSuffixRegex = regexp.MustCompile(`(?i)\s*[-–]\s*(near\s+mint|mint|nm|lp|mp|hp|damaged)\b.*$`)

// ExtractCardNameFromTitle strips grader/grade and condition patterns from a title
// to produce a cleaner card name when tags-based extraction is unavailable.
func ExtractCardNameFromTitle(title string) string {
	cleaned := titleCleanupRegex.ReplaceAllString(title, "")
	cleaned = conditionSuffixRegex.ReplaceAllString(cleaned, "")
	cleaned = strings.Join(strings.Fields(cleaned), " ")
	cleaned = strings.TrimSpace(cleaned)
	if cleaned == "" {
		return title
	}
	return cleaned
}

var graderGradeRegex = regexp.MustCompile(`(?i)\b(CGC|BGS|PSA|SGC)\s*(\d{1,2}(?:\.\d)?)\b`)

// ExtractGraderAndGrade extracts a grading company and numeric grade from a title string.
// Returns empty string and 0 if no match is found.
func ExtractGraderAndGrade(title string) (string, float64) {
	matches := graderGradeRegex.FindStringSubmatch(title)
	if len(matches) < 3 {
		return "", 0
	}
	grader := strings.ToUpper(matches[1])
	grade, err := strconv.ParseFloat(matches[2], 64)
	if err != nil || grade < 1 || grade > 10 {
		return "", 0
	}
	return grader, grade
}

// OrdersExportRow represents a single row parsed from an orders export CSV.
type OrdersExportRow struct {
	OrderNumber  string
	Date         string      // YYYY-MM-DD
	SalesChannel SaleChannel // Mapped from CSV value
	ProductTitle string
	Grader       string
	CertNumber   string
	Grade        float64
	UnitPrice    float64 // Dollars
}

// OrdersImportResult categorizes parsed order rows by match status.
type OrdersImportResult struct {
	Matched     []OrdersImportMatch `json:"matched"`
	AlreadySold []OrdersImportSkip  `json:"alreadySold"`
	NotFound    []OrdersImportSkip  `json:"notFound"`
	Skipped     []OrdersImportSkip  `json:"skipped"`
}

// OrdersImportMatch represents a CSV row matched to an unsold inventory purchase.
type OrdersImportMatch struct {
	CertNumber           string      `json:"certNumber"`
	ProductTitle         string      `json:"productTitle"`
	SaleChannel          SaleChannel `json:"saleChannel"`
	SaleDate             string      `json:"saleDate"`
	SalePriceCents       int         `json:"salePriceCents"`
	SaleFeeCents         int         `json:"saleFeeCents"`
	PurchaseID           string      `json:"purchaseId"`
	CampaignID           string      `json:"campaignId"`
	CardName             string      `json:"cardName"`
	BuyCostCents         int         `json:"buyCostCents"`
	NetProfitCents       int         `json:"netProfitCents"`
	CampaignLookupFailed bool        `json:"campaignLookupFailed,omitempty"`
}

// OrdersImportSkip represents a CSV row that was skipped or couldn't be matched.
type OrdersImportSkip struct {
	CertNumber   string `json:"certNumber"`
	ProductTitle string `json:"productTitle"`
	Reason       string `json:"reason"` // "already_sold", "not_found", "duplicate", "not_psa", "unknown_channel"
}

// OrdersConfirmItem carries the data needed to create a sale from a confirmed import match.
type OrdersConfirmItem struct {
	PurchaseID     string      `json:"purchaseId"`
	SaleChannel    SaleChannel `json:"saleChannel"`
	SaleDate       string      `json:"saleDate"`
	SalePriceCents int         `json:"salePriceCents"`
	OrderID        string      `json:"orderId,omitempty"`
}

// MMExportEntry represents a single row in the Market Movers collection import CSV format.
// The 17-column format matches Market Movers' collection export/import schema.
type MMExportEntry struct {
	Sport                string  // Column 1: Sport/category (e.g. "Pokemon", "Basketball")
	Grade                string  // Column 2: e.g. "PSA 10", "BGS 9.5"
	PlayerName           string  // Column 3: Player/subject name
	Year                 string  // Column 4: Card year
	Set                  string  // Column 5: Set name
	Variation            string  // Column 6: Card variation
	CardNumber           string  // Column 7: Card number with "#" prefix
	SpecificQualifier    string  // Column 8: Always empty
	Quantity             string  // Column 9: Always "1"
	DatePurchased        string  // Column 10: YYYY-MM-DD
	PurchasePricePerCard float64 // Column 11: Buy cost in dollars
	Notes                string  // Column 12: Cert number for traceability
	Category             string  // Column 13: Always empty
	DateSold             string  // Column 14: Empty (unsold)
	SoldPricePerCard     string  // Column 15: Empty (unsold)
	LastSalePrice        float64 // Column 16: MM or CL value in dollars (best available)
	LastSaleDate         string  // Column 17: Last sale date — left blank on export (we don't know the actual last-sale date)
}

// MMRefreshRow holds the fields needed to refresh mm_value_cents from a Market Movers CSV row.
// The cert number comes from the "Notes" column (col 12) and the price from "Last Sale Price" (col 16).
type MMRefreshRow struct {
	CertNumber    string  // From "Notes" column
	LastSalePrice float64 // From "Last Sale Price" column, in dollars
}

// MMRefreshItemResult contains per-row outcome for MM value refreshes.
type MMRefreshItemResult struct {
	CertNumber    string `json:"certNumber"`
	CardName      string `json:"cardName,omitempty"`
	OldValueCents int    `json:"oldValueCents"`
	NewValueCents int    `json:"newValueCents"`
	Status        string `json:"status"` // "updated", "skipped", "failed"
	Error         string `json:"error,omitempty"`
}

// MMRefreshResult summarizes the outcome of a Market Movers value refresh.
type MMRefreshResult struct {
	Updated  int                   `json:"updated"`
	NotFound int                   `json:"notFound"`
	Skipped  int                   `json:"skipped"`
	Failed   int                   `json:"failed"`
	Errors   []ImportError         `json:"errors,omitempty"`
	Results  []MMRefreshItemResult `json:"results,omitempty"`
}

// cardNumberPattern matches expected card number formats (digits, alphanumeric, with optional slash separator).
var cardNumberPattern = regexp.MustCompile(`^[A-Za-z0-9]+([/\-][A-Za-z0-9]+)?$`)

// ParseShopifyTags extracts card metadata from a Shopify product tags string.
// Tags format: "cardName, cardNumber, setName, sport" (comma-separated, positional).
// Returns an error if the tags are malformed (empty cardName, invalid cardNumber pattern,
// or more than 4 parts). Missing trailing parts are treated as empty strings.
func ParseShopifyTags(tags string) (cardName, cardNumber, setName, sport string, err error) {
	if strings.TrimSpace(tags) == "" {
		return "", "", "", "", fmt.Errorf("empty tags string")
	}
	parts := strings.Split(tags, ",")
	if len(parts) > 4 {
		return "", "", "", "", fmt.Errorf("too many tag parts: got %d, max 4", len(parts))
	}
	for i := range parts {
		parts[i] = strings.TrimSpace(parts[i])
	}
	cardName = parts[0]
	if cardName == "" {
		return "", "", "", "", fmt.Errorf("card name is empty in tags")
	}
	if len(parts) > 1 {
		cardNumber = parts[1]
	}
	if len(parts) > 2 {
		setName = parts[2]
	}
	if len(parts) > 3 {
		sport = parts[3]
	}
	if cardNumber != "" && !cardNumberPattern.MatchString(cardNumber) {
		return "", "", "", "", fmt.Errorf("invalid card number format: %q", cardNumber)
	}
	return cardName, cardNumber, setName, sport, nil
}
