// proxyFromEnv maps a PSA_PORTAL_PROXY_URL string to a Playwright context
// `proxy` option. Returns undefined when unset so egress stays direct.
// Format: http://user:pass@host:port or socks5://host:port.
export function proxyFromEnv(url) {
  if (!url) return undefined;
  const u = new URL(url);
  return {
    server: `${u.protocol}//${u.host}`,
    username: u.username || undefined,
    password: u.password ? decodeURIComponent(u.password) : undefined,
  };
}
