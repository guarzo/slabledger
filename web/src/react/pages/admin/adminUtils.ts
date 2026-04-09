/**
 * Format an ISO timestamp for display in admin panels.
 * Returns "-" for null, undefined, empty, or unparseable values.
 * Output example: "Apr 7, 2026, 9:00 AM"
 */
export function formatAdminDate(ts: string | null | undefined): string {
  if (!ts) return '-';
  const d = new Date(ts);
  if (isNaN(d.getTime())) return '-';
  return new Intl.DateTimeFormat('en-US', {
    month: 'short',
    day: 'numeric',
    year: 'numeric',
    hour: 'numeric',
    minute: '2-digit',
  }).format(d);
}
