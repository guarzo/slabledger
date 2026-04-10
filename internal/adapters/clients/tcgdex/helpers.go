package tcgdex

import (
	"context"
	"fmt"
	"net/url"
	"strings"

	apperrors "github.com/guarzo/slabledger/internal/domain/errors"
)

const defaultBaseURL = "https://api.tcgdex.net/v2"

// langToDisplay maps a TCGdex language code to a display name.
func langToDisplay(lang string) string {
	switch lang {
	case "en":
		return "English"
	case "ja":
		return "Japanese"
	case "fr":
		return "French"
	case "es":
		return "Spanish"
	case "it":
		return "Italian"
	case "pt-br":
		return "Portuguese"
	case "de":
		return "German"
	case "id":
		return "Indonesian"
	case "th":
		return "Thai"
	default:
		return lang
	}
}

// buildURL constructs a TCGdex API URL for the given language and path.
func (t *TCGdex) buildURL(lang, path string) string {
	path = strings.TrimLeft(path, "/")
	return fmt.Sprintf("%s/%s/%s", t.baseURL, lang, path)
}

// escapePathSegment encodes a single path segment for use in URLs.
// In addition to standard path escaping, it encodes "+" to "%2B" because
// some servers (including TCGdex) interpret "+" as a space in path segments.
func escapePathSegment(segment string) string {
	return strings.ReplaceAll(url.PathEscape(segment), "+", "%2B")
}

// get performs an HTTP GET request and decodes JSON into the target.
func (t *TCGdex) get(ctx context.Context, u string, into any) error {
	if err := t.rateLimiter.Wait(ctx); err != nil {
		return apperrors.ProviderUnavailable("TCGdex", fmt.Errorf("rate limiter: %w", err))
	}
	headers := make(map[string]string)
	return t.httpClient.GetJSON(ctx, u, headers, tcgdexTimeout, into)
}
