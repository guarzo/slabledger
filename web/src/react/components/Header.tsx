/**
 * Header Component
 *
 * Sticky header with logo, navigation, and user dropdown menu.
 */
import { lazy, Suspense, useEffect, useState } from 'react';
import { Link } from 'react-router-dom';
import { DropdownMenu } from 'radix-ui';
import Navigation from './Navigation';
import StatusIndicator from './StatusIndicator';
import { useAuth } from '../contexts/AuthContext';
import logoSrc from '../../assets/logo.png';

const PriceLookupDrawer = lazy(() => import('./PriceLookupDrawer'));

function UserInitial({ name }: { name: string }) {
  const initial = name.charAt(0).toUpperCase();
  return (
    <div className="w-8 h-8 rounded-full bg-[var(--brand-500)] flex items-center justify-center text-white text-sm font-bold">
      {initial}
    </div>
  );
}

export default function Header() {
  const { user, loading, logout } = useAuth();
  const [isScrolled, setIsScrolled] = useState(false);
  const [avatarError, setAvatarError] = useState(false);
  const [lookupOpen, setLookupOpen] = useState(false);

  useEffect(() => {
    setAvatarError(false);
  }, [user]);

  useEffect(() => {
    const handleScroll = () => setIsScrolled(window.scrollY > 10);
    handleScroll();
    window.addEventListener('scroll', handleScroll);
    return () => window.removeEventListener('scroll', handleScroll);
  }, []);

  return (
    <header
      className={`
        sticky top-0 z-50
        bg-[var(--surface-1)]/80
        border-b border-[rgba(255,255,255,0.06)]
        backdrop-blur-xl
        transition-all duration-300
        ${isScrolled ? 'shadow-md' : 'shadow-sm'}
      `}
      role="banner"
    >
      <div className="flex items-center justify-between gap-6 px-6 py-2.5 max-w-[1920px] mx-auto">
        {/* Logo Section */}
        <Link
          to="/"
          className="flex items-center gap-2 group relative"
        >
          <img
            src={logoSrc}
            alt="SlabLedger"
            className="h-7 w-7 rounded-lg object-contain transition-transform duration-300 group-hover:scale-105"
          />
          <div className="hidden sm:block">
            <div className="text-sm font-semibold text-[var(--text)] leading-tight tracking-tight">
              SlabLedger
            </div>
          </div>
        </Link>

        {/* Navigation - Centered */}
        <div className="flex-1 flex justify-center max-w-4xl min-w-0">
          <Navigation />
        </div>

        {/* Right Actions */}
        <div className="flex gap-3 items-center">
          <button
            type="button"
            onClick={() => setLookupOpen(true)}
            className="p-2 rounded-[var(--radius-md)] text-[var(--text-muted)] hover:text-[var(--text)] hover:bg-[var(--surface-2)]/60 transition-colors"
            aria-label="Price Lookup"
            title="Price Lookup"
          >
            <svg width="18" height="18" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2" strokeLinecap="round" strokeLinejoin="round" aria-hidden="true" focusable="false">
              <circle cx="11" cy="11" r="8" />
              <line x1="21" y1="21" x2="16.65" y2="16.65" />
            </svg>
          </button>
          <StatusIndicator />
          {loading ? (
            <span className="text-xs text-[var(--text-muted)]">...</span>
          ) : user ? (
            <DropdownMenu.Root>
              <DropdownMenu.Trigger asChild>
                <button
                  type="button"
                  className="flex items-center gap-2 px-2 py-1.5 rounded-[var(--radius-md)] hover:bg-[var(--surface-2)]/60 transition-colors"
                  aria-label={`User menu for ${user.username}`}
                >
                  {user.avatar_url && !avatarError ? (
                    <img
                      src={user.avatar_url}
                      alt={`${user.username} avatar`}
                      className="w-8 h-8 rounded-full border border-[var(--surface-2)]"
                      onError={() => setAvatarError(true)}
                    />
                  ) : (
                    <UserInitial name={user.username} />
                  )}
                  <span className="hidden md:block text-sm text-[var(--text-muted)] max-w-[100px] truncate">
                    {user.username}
                  </span>
                  <svg className="w-3.5 h-3.5 text-[var(--text-muted)]" fill="none" stroke="currentColor" viewBox="0 0 24 24">
                    <path strokeLinecap="round" strokeLinejoin="round" strokeWidth={2} d="M19 9l-7 7-7-7" />
                  </svg>
                </button>
              </DropdownMenu.Trigger>
              <DropdownMenu.Portal>
                <DropdownMenu.Content
                  align="end"
                  sideOffset={4}
                  className="w-40 py-1 bg-[var(--surface-1)] border border-[var(--surface-2)]
                             rounded-[var(--radius-md)] shadow-[var(--shadow-2)] z-50
                             data-[state=open]:animate-[fadeIn_150ms_ease-out]
                             data-[state=closed]:animate-[fadeOut_100ms_ease-in]"
                >
                  <DropdownMenu.Item
                    onSelect={logout}
                    className="w-full text-left px-4 py-2 text-sm text-[var(--text-muted)]
                               hover:bg-[var(--surface-2)]/60 hover:text-[var(--danger)]
                               data-[highlighted]:bg-[var(--surface-2)]/60
                               data-[highlighted]:text-[var(--danger)]
                               transition-colors flex items-center gap-2 outline-none cursor-default"
                  >
                    <svg className="w-4 h-4" fill="none" stroke="currentColor" viewBox="0 0 24 24">
                      <path strokeLinecap="round" strokeLinejoin="round" strokeWidth={2} d="M17 16l4-4m0 0l-4-4m4 4H7m6 4v1a3 3 0 01-3 3H6a3 3 0 01-3-3V7a3 3 0 013-3h4a3 3 0 013 3v1" />
                    </svg>
                    Log out
                  </DropdownMenu.Item>
                </DropdownMenu.Content>
              </DropdownMenu.Portal>
            </DropdownMenu.Root>
          ) : (
            <Link
              to="/login"
              className="px-4 py-2 text-sm font-medium text-white bg-[var(--brand-500)] rounded-[var(--radius-md)] hover:bg-[var(--brand-600)] transition-colors"
            >
              Sign In
            </Link>
          )}
        </div>
      </div>

      {lookupOpen && (
        <Suspense fallback={null}>
          <PriceLookupDrawer open={lookupOpen} onOpenChange={setLookupOpen} />
        </Suspense>
      )}
    </header>
  );
}
