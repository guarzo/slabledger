// Only initial navigation is retryable: callers must not wrap the SSO flow.
export async function gotoStart(page, url) {
  const options = { waitUntil: 'domcontentloaded', timeout: 60000 };
  try {
    return await page.goto(url, options);
  } catch (error) {
    const code = error.message.match(/\bnet::(ERR_[A-Z_]+)\b/)?.[1];
    const allowed = ['ERR_TUNNEL_CONNECTION_FAILED', 'ERR_PROXY_CONNECTION_FAILED',
      'ERR_CONNECTION_RESET', 'ERR_CONNECTION_CLOSED', 'ERR_EMPTY_RESPONSE', 'ERR_NETWORK_CHANGED'];
    if (error.name === 'TimeoutError' || !allowed.includes(code)) throw error;
    console.error(`harvest-psa-token: initial navigation ${code}; retrying once after 5s`);
    await page.waitForTimeout(5000);
    return page.goto(url, options);
  }
}

export function safePageURL(value) {
  try {
    const url = new URL(value);
    if (url.protocol !== 'http:' && url.protocol !== 'https:') return `[${url.protocol}]`;
    return url.origin + url.pathname;
  } catch {
    return '[invalid-url]';
  }
}

// Snapshot suffixes/children contain field values. Read ONLY the role/name header.
export async function logNoPasswordSummary(page, secrets) {
  const summary = { time: new Date().toISOString(), phase: 'no-password-field' };
  const redactions = secrets.filter(Boolean).flatMap((s) => [s, encodeURIComponent(s)])
    .sort((a, b) => b.length - a.length).map((s) => s.replace(/[.*+?^${}()|[\]\\]/g, '\\$&')).join('|');
  const pattern = redactions ? new RegExp(redactions, 'gi') : null;
  const clean = (value) => {
    if (pattern) value = value.replace(pattern, '[REDACTED]');
    return value.replace(/https?:\/\/[^\s"'<>]+/g, safePageURL).replace(/[?#][^\s"'<>]*/g, '').slice(0, 80);
  };
  let timer;
  try {
    const collect = async () => {
      const controls = [];
      const deadline = Date.now() + 5000;
      for (const el of await page.locator('input, button, [role="button"]').all()) {
        if (Date.now() >= deadline) break;
        if (!await el.isVisible()) continue;
        const attrs = await el.evaluate((node) => Object.fromEntries(
          ['type', 'name', 'id', 'role'].map((key) => [key, node.getAttribute(key) || '']),
        ), undefined, { timeout: 1000 });
        const header = (await el.ariaSnapshot({ timeout: 1000 })).split('\n')[0]
          .replace(/^- '((?:[^']|'')*)'(?=:|$)/, (_, key) => `- ${key.replaceAll("''", "'")}`);
        const match = header.match(/^- ([\w-]+)(?: ("(?:[^"\\]|\\.)*"))?/);
        controls.push({ ...attrs, role: attrs.role || match?.[1] || '', accessibleName: JSON.parse(match?.[2] || '""') });
        if (controls.length === 20) break;
      }
      return { url: safePageURL(page.url()), title: await page.title(), controls };
    };
    Object.assign(summary, await Promise.race([
      collect(), new Promise((_, reject) => { timer = setTimeout(() => reject(new Error('diagnostic deadline')), 5000); }),
    ]));
  } catch {
    summary.unavailable = true; // Never replace the login error or log a raw diagnostic error.
  } finally {
    clearTimeout(timer);
  }
  const serialize = () => JSON.stringify(summary, (_, value) => typeof value === 'string' ? clean(value) : value);
  while (summary.controls?.length && Buffer.byteLength(serialize()) > 3500) summary.controls.pop();
  console.error(`harvest-psa-token: ${serialize()}`);
}
