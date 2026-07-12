package psaportal

import (
	"context"
	"fmt"
	"strings"

	"github.com/guarzo/slabledger/internal/domain/inventory"
	"github.com/guarzo/slabledger/internal/domain/observability"
)

// mapRows converts flattened Lightdash rows into PSAExportRows. One malformed
// row must not abort the whole sync; log and skip it, matching the CSV import
// path (importPSARows). Rows without a cert number are silently dropped.
func mapRows(ctx context.Context, raw []map[string]string, logger observability.Logger) ([]inventory.PSAExportRow, error) {
	rows := make([]inventory.PSAExportRow, 0, len(raw))
	for _, r := range raw {
		m, err := mapRow(r)
		if err != nil {
			logger.Warn(ctx, "psaportal: skipping malformed row", observability.Err(err))
			continue
		}
		if m.CertNumber == "" {
			continue
		}
		rows = append(rows, m)
	}
	if len(raw) > 0 && len(rows) == 0 {
		return nil, fmt.Errorf("psaportal: all %d rows failed to map", len(raw))
	}
	return rows, nil
}

// parseEmbedURL splits "https://host/embed/{projectUUID}#{jwt}".
func parseEmbedURL(u string) (projectUUID, jwt string, err error) {
	base, token, found := strings.Cut(u, "#")
	if !found {
		return "", "", fmt.Errorf("psaportal: embed url missing token: %q", u)
	}
	jwt = token
	base = strings.TrimRight(base, "/")
	seg := strings.Split(base, "/")
	projectUUID = seg[len(seg)-1]
	if projectUUID == "" || jwt == "" {
		return "", "", fmt.Errorf("psaportal: cannot parse embed url: %q", u)
	}
	return projectUUID, jwt, nil
}
