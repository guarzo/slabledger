import { describe, it, expect } from 'vitest';
import { proxyFromEnv } from './proxy-from-env.mjs';

describe('proxyFromEnv', () => {
  const cases = [
    {
      name: 'http with credentials',
      input: 'http://user:pass@host:10001',
      want: { server: 'http://host:10001', username: 'user', password: 'pass' },
    },
    {
      name: 'socks5 without credentials',
      input: 'socks5://host:1080',
      want: { server: 'socks5://host:1080', username: undefined, password: undefined },
    },
    {
      name: 'percent-encoded credentials are decoded',
      input: 'http://user%40name:pa%3Ass@host:10001',
      want: { server: 'http://host:10001', username: 'user@name', password: 'pa:ss' },
    },
    { name: 'empty string', input: '', want: undefined },
    { name: 'undefined', input: undefined, want: undefined },
  ];
  for (const c of cases) {
    it(c.name, () => {
      expect(proxyFromEnv(c.input)).toEqual(c.want);
    });
  }
});
