// Auto-retry policy for cert import.
//
// A transient PSA failure used to leave rows staged with a message telling the
// operator to press Import again. That is work the screen can do itself: the
// backend already classifies each per-cert error as transient or permanent
// (`retryable`), so the only thing a human added was the waiting (SLA-108).
//
// The schedule is tuned to the backend's circuit breaker, whose Timeout is 120s
// (resilience.DefaultCircuitBreakerConfig). Retrying inside that window only
// collects instant ERR_PROV_CIRCUIT_OPEN responses, so the last attempt lands
// after the breaker has had a chance to go half-open. Attempts are bounded —
// PSA's daily quota does not reset within any window a browser tab should sit
// spinning through, and pretending otherwise would be the same dishonesty as
// the message this replaces.
export const AUTO_RETRY_DELAYS_MS = [15_000, 60_000, 150_000] as const;
export const MAX_AUTO_RETRIES = AUTO_RETRY_DELAYS_MS.length;

// autoRetryDelayMs returns how long to wait before automatic attempt `attempt`
// (1-based), or null once the attempts are spent.
export function autoRetryDelayMs(attempt: number): number | null {
  if (attempt < 1 || attempt > MAX_AUTO_RETRIES) return null;
  return AUTO_RETRY_DELAYS_MS[attempt - 1];
}

function plural(n: number): string {
  return n === 1 ? '' : 's';
}

// retryPendingMessage describes a retry that is already scheduled. It must not
// ask the operator to do anything — that is the whole point.
export function retryPendingMessage(count: number, attempt: number, delayMs: number): string {
  const secs = Math.round(delayMs / 1000);
  return (
    `PSA didn't answer for ${count} cert${plural(count)}. ` +
    `Retrying automatically in ${secs}s (attempt ${attempt} of ${MAX_AUTO_RETRIES}). ` +
    `Nothing was lost — you don't need to do anything.`
  );
}

// batchRetryPendingMessage covers the whole-batch failure (network error, 500):
// no per-cert verdict came back, so the detail is whatever the request threw.
export function batchRetryPendingMessage(detail: string, attempt: number, delayMs: number): string {
  const secs = Math.round(delayMs / 1000);
  return (
    `${detail}. Retrying automatically in ${secs}s (attempt ${attempt} of ${MAX_AUTO_RETRIES}). ` +
    `Nothing was lost — you don't need to do anything.`
  );
}

// retryExhaustedMessage is the honest terminal state: automatic recovery is
// spent, the rows are still safe, and now a human decision is genuinely needed.
export function retryExhaustedMessage(count: number): string {
  return (
    `PSA is still unavailable after ${MAX_AUTO_RETRIES} automatic retries, so ` +
    `${count} cert${plural(count)} couldn't be imported. They're staged and nothing was lost. ` +
    `This usually means the daily PSA quota is spent — press Import to try again, ` +
    `or leave them staged until tomorrow.`
  );
}
