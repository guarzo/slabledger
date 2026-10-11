import { expect, it, vi } from 'vitest';
import { resetAuthContext, AUTH_CONTEXT_RESET_EVENT } from './authContextEvents';
it('clears persisted recovery even before the lazy Shows module has registered an owner', () => {
  sessionStorage.clear(); sessionStorage.setItem('slabledger:show-scans:v1:7', 'pending'); sessionStorage.setItem('other-feature', 'keep');
  const listener = vi.fn(); window.addEventListener(AUTH_CONTEXT_RESET_EVENT, listener);
  resetAuthContext();
  expect(sessionStorage.getItem('slabledger:show-scans:v1:7')).toBeNull(); expect(sessionStorage.getItem('other-feature')).toBe('keep'); expect(listener).toHaveBeenCalledTimes(1);
  window.removeEventListener(AUTH_CONTEXT_RESET_EVENT, listener);
});
