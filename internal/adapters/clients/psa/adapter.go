package psa

import (
	"context"
	"fmt"

	"github.com/guarzo/slabledger/internal/domain/inventory"
)

// CertAdapter wraps a PSA Client to implement inventory.CertLookup.
type CertAdapter struct {
	client *Client
}

var _ inventory.CertLookup = (*CertAdapter)(nil)

// NewCertAdapter creates a CertLookup adapter around a PSA client.
func NewCertAdapter(client *Client) *CertAdapter {
	return &CertAdapter{client: client}
}

// LookupCert resolves a PSA certificate number to card details.
func (a *CertAdapter) LookupCert(ctx context.Context, certNumber string) (*inventory.CertInfo, error) {
	info, err := a.client.GetCert(ctx, certNumber)
	if err != nil {
		return nil, fmt.Errorf("PSA cert lookup %s: %w", certNumber, err)
	}

	grade := ParseGrade(info.CardGrade)
	if grade == 0 {
		grade = ParseGrade(info.GradeDescription)
	}

	return &inventory.CertInfo{
		CertNumber: info.CertNumber,
		CardName:   BuildCardName(info),
		Grade:      grade, // float64 from ParseGrade
		Year:       info.Year,
		Brand:      info.Brand,
		Category:   info.Category,
		Subject:    info.Subject,
		Variety:    info.Variety,
		CardNumber: info.CardNumber,
		Population: info.TotalPopulation,
		PopHigher:  info.PopulationHigher,
	}, nil
}
