/**
 * Appends a cache-busting query parameter to external image URLs so that
 * CloudFront (and similar CDNs) treat the CORS request as a separate cache
 * key from any previously cached non-CORS response.
 *
 * Without this, `crossOrigin="anonymous"` on `<img>` tags can intermittently
 * fail: if the CDN edge node already has a cached response without
 * `Access-Control-Allow-Origin` (from a prior non-CORS request), the browser
 * blocks the image entirely.
 *
 * Only applied to external URLs (http/https); local/relative URLs pass through.
 */
export function corsUrl(url: string | undefined): string | undefined {
  if (!url || !url.startsWith('http')) return url;
  const sep = url.includes('?') ? '&' : '?';
  return `${url}${sep}x-cors=1`;
}
