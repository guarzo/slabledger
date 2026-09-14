package cardladder

import (
	"context"
	"errors"
	"math"
	"net/url"
	"strconv"
	"strings"
	"time"

	apperrors "github.com/guarzo/slabledger/internal/domain/errors"
	"github.com/guarzo/slabledger/internal/domain/mathutil"
	"github.com/guarzo/slabledger/internal/domain/observability"
	sp "github.com/guarzo/slabledger/internal/domain/showprep"
	"github.com/guarzo/slabledger/internal/platform/cardutil"
	"golang.org/x/sync/singleflight"
)

// ShowPrepSource shares the existing configured client, token manager and limiter.
// It does not read/write the legacy 90-day comp pipeline or financial records.
type ShowPrepSource struct {
	client *Client
	group  singleflight.Group
	logger observability.Logger
}

var _ sp.Source = (*ShowPrepSource)(nil)

func NewShowPrepSource(client *Client, logger observability.Logger) *ShowPrepSource {
	return &ShowPrepSource{client: client, logger: logger}
}
func (s *ShowPrepSource) Fetch(ctx context.Context, id sp.Identity, now time.Time) (sp.Snapshot, error) {
	if err := ctx.Err(); err != nil {
		return sp.Snapshot{}, err
	}
	start, end := sp.Window(now)
	ch := s.group.DoChan(id.Key()+start+end, func() (any, error) { return s.fetch(ctx, id, now) })
	select {
	case <-ctx.Done():
		return sp.Snapshot{}, ctx.Err()
	case result := <-ch:
		snapshot := result.Val.(sp.Snapshot)
		snapshot.Sales = append([]sp.Sale{}, snapshot.Sales...)
		return snapshot, result.Err
	}
}

type showPrepSale struct {
	SaleComp
	Currency string `json:"currency"`
}
type showPrepPage struct {
	Hits      []showPrepSale `json:"hits"`
	TotalHits *int           `json:"totalHits"`
}

func (s *ShowPrepSource) fetch(ctx context.Context, id sp.Identity, now time.Time) (sp.Snapshot, error) {
	started := time.Now()
	start, end := sp.Window(now)
	snapshot := sp.Snapshot{Identity: id, Source: "cardladder", WindowStart: start, WindowEnd: end, Sales: []sp.Sale{}}
	fail := func(reason string, err error) (sp.Snapshot, error) {
		snapshot.AttemptError = reason
		if s.logger != nil {
			s.logger.Warn(ctx, "show preparation source incomplete", observability.String("identity", id.Key()), observability.String("reason", reason), observability.Int("sales", len(snapshot.Sales)), observability.Duration("elapsed", time.Since(started)))
		}
		return snapshot, err
	}
	if s.client == nil {
		return fail("CardLadder credentials unavailable", errors.New("client unavailable"))
	}
	if !id.Valid() {
		return fail("Comparable identity unresolved", nil)
	}
	condition := cardutil.GradeToCondition(id.Grade)
	total := -1
	consumed := 0
	previousDate := ""
	seen := map[string]sp.Sale{}
	firstSeenPage := map[string]int{}
	pages := map[string]bool{}
	for page := 0; page < 5; page++ {
		if err := ctx.Err(); err != nil {
			return fail("CardLadder source timeout", err)
		}
		params := url.Values{"index": {"salesarchive"}, "query": {""}, "page": {strconv.Itoa(page)}, "limit": {"100"}, "sort": {"date"}, "direction": {"desc"}, "filters": {"condition:" + condition + "|profileId:" + id.ProfileID + "|gradingCompany:" + strings.ToLower(id.Grader)}}
		var response showPrepPage
		if err := s.client.doGet(ctx, params, &response); err != nil {
			reason := "CardLadder refresh failed"
			switch {
			case ctx.Err() != nil:
				reason = "CardLadder source timeout"
			case apperrors.HasErrorCode(err, apperrors.ErrCodeProviderRateLimit):
				reason = "CardLadder rate limit reached"
			case apperrors.HasErrorCode(err, apperrors.ErrCodeConfigMissing):
				reason = "CardLadder credentials unavailable"
			}
			return fail(reason, err)
		}
		if response.TotalHits == nil || *response.TotalHits < 0 || (total >= 0 && total != *response.TotalHits) {
			return fail("Unstable CardLadder pagination", nil)
		}
		total = *response.TotalHits
		if len(response.Hits) > 100 || consumed+len(response.Hits) > total {
			return fail("Invalid CardLadder pagination bounds", nil)
		}
		pageKey := sp.Fingerprint(response.Hits)
		if pages[pageKey] {
			return fail("Repeated CardLadder page", nil)
		}
		pages[pageKey] = true
		crossedCutoff := false
		overlap := false
		for _, record := range response.Hits {
			sale, reason := qualifiedShowSale(record, id, end)
			if reason != "" {
				return fail(reason, nil)
			}
			if previousDate != "" && sale.Date > previousDate {
				return fail("CardLadder date ordering is not newest first", nil)
			}
			previousDate = sale.Date
			if old, ok := seen[sale.ID]; ok {
				if old != sale {
					return fail("Contradictory duplicate CardLadder sale", nil)
				}
				if firstSeenPage[sale.ID] != page {
					overlap = true
				}
				continue
			}
			seen[sale.ID] = sale
			firstSeenPage[sale.ID] = page
			if sale.Date < start {
				crossedCutoff = true
				continue
			}
			snapshot.Sales = append(snapshot.Sales, sale)
		}
		// Keep this page's inspectable records, but overlapping page boundaries
		// cannot prove coverage even when raw row counts reach totalHits.
		if overlap {
			return fail("Overlapping CardLadder pages; coverage uncertain", nil)
		}
		consumed += len(response.Hits)
		if consumed == total || crossedCutoff {
			snapshot.Complete = true
			snapshot.RefreshedAt = time.Now().UTC()
			if s.logger != nil {
				s.logger.Info(ctx, "show preparation source complete", observability.String("identity", id.Key()), observability.Int("pages", page+1), observability.Int("sales", len(snapshot.Sales)), observability.Duration("elapsed", time.Since(started)))
			}
			return snapshot, nil
		}
		if len(response.Hits) < 100 {
			return fail("Missing CardLadder page coverage", nil)
		}
	}
	return fail("CardLadder five-page budget exhausted", nil)
}
func qualifiedShowSale(record showPrepSale, id sp.Identity, end string) (sp.Sale, string) {
	if record.ProfileID != id.ProfileID || !strings.EqualFold(strings.TrimSpace(record.GradingCompany), id.Grader) {
		return sp.Sale{}, "Comparable identity mismatch"
	}
	condition := strings.TrimSpace(record.Condition)
	if condition != cardutil.GradeToCondition(id.Grade) && !strings.EqualFold(condition, id.Condition()) {
		return sp.Sale{}, "Comparable grade mismatch"
	}
	if record.ItemID == "" {
		return sp.Sale{}, "Missing CardLadder sale ID"
	}
	if math.IsNaN(record.Price) || math.IsInf(record.Price, 0) || record.Price <= 0 || record.Price > float64(math.MaxInt64/1000) {
		return sp.Sale{}, "Invalid CardLadder sale amount"
	}
	if record.Currency != "" && !strings.EqualFold(strings.TrimSpace(record.Currency), "USD") {
		return sp.Sale{}, "Unsupported CardLadder sale currency"
	}
	date, err := time.Parse(time.RFC3339Nano, record.Date)
	if err != nil {
		date, err = time.Parse(time.DateOnly, record.Date)
	}
	if err != nil || date.UTC().Format(time.DateOnly) > end {
		return sp.Sale{}, "Invalid or future CardLadder sale date"
	}
	cents := mathutil.ToCentsInt(record.Price)
	if cents <= 0 {
		return sp.Sale{}, "Invalid CardLadder sale amount"
	}
	link := ""
	u, err := url.Parse(record.URL)
	if err == nil && u.Host != "" && (u.Scheme == "http" || u.Scheme == "https") {
		link = u.String()
	}
	return sp.Sale{ID: record.ItemID, Date: date.UTC().Format(time.DateOnly), PriceCents: cents, Platform: record.Platform, URL: link, ListingType: record.ListingType}, ""
}
