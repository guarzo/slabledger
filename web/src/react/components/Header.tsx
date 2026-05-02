/**
 * Header Component
 *
 * Sticky header with logo, navigation, and user dropdown menu.
 */
import { useCallback, useEffect, useState } from 'react';
import { Link, useLocation } from 'react-router-dom';
import { DropdownMenu } from 'radix-ui';
import Navigation from './Navigation';
import StatusIndicator from './StatusIndicator';
import { useAuth } from '../contexts/AuthContext';
import CardShell from '../ui/CardShell';
import logoSrc from '../../assets/logo.png';

const NAV_LABELS: { path: string; label: string }[] = [
  { path: '/', label: 'Dashboard' },
  { path: '/inventory', label: 'Inventory' },
  { path: '/campaigns', label: 'Campaigns' },
  { path: '/insights', label: 'Insights' },
  { path: '/scan', label: 'Scan' },
  { path: '/reprice', label: 'Reprice' },
  { path: '/invoices', label: 'Invoices' },
  { path: '/admin', label: 'Admin' },
];

function pageLabelFor(pathname: string): string {
  // Longest-prefix match wins so /campaigns/123 picks "Campaigns".
  const match = NAV_LABELS
    .filter((item) => item.path === '/' ? pathname === '/' : pathname === item.path || pathname.startsWith(item.path + '/'))
    .sort((a, b) => b.path.length - a.path.length)[0];
  return match?.label ?? 'Menu';
}

function UserInitial({ name }: { name: string }) {
  const initial = name.charAt(0).toUpperCase();
  return (
    <div className="w-8 h-8 rounded-full bg-[var(--surface-2)] flex items-center justify-center text-[var(--text)] text-sm font-bold">
      {initial}
    </div>
  );
}

export default function Header() {
  const { user, loading, logout } = useAuth();
  const [isScrolled, setIsScrolled] = useState(false);
  const [avatarError, setAvatarError] = useState(false);
  const [menuOpen, setMenuOpen] = useState(false);
  const closeMenu = useCallback(() => setMenuOpen(false), []);
  const location = useLocation();
  const currentPageLabel = pageLabelFor(location.pathname);

  useEffect(() => {
    setMenuOpen(false);
  }, [location.pathname]);

  useEffect(() => {
    if (!menuOpen) return;
    const onKey = (e: KeyboardEvent) => { if (e.key === 'Escape') setMenuOpen(false); };
    document.addEventListener('keydown', onKey);
    return () => document.removeEventListener('keydown', onKey);
  }, [menuOpen]);

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

        {/* Hamburger button - mobile only */}
        <button
          type="button"
          className="md:hidden flex flex-col items-center gap-0.5 px-2 py-1.5 rounded-[var(--radius-md)] text-[var(--text-muted)] hover:text-[var(--text)] hover:bg-[var(--surface-2)]/60 transition-colors focus-ring"
          onClick={() => setMenuOpen((prev) => !prev)}
          aria-label={menuOpen ? 'Close menu' : 'Open menu'}
          aria-expanded={menuOpen}
        >
          {menuOpen ? (
            <svg width="20" height="20" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2" strokeLinecap="round" strokeLinejoin="round" aria-hidden="true" focusable="false">
              <line x1="18" y1="6" x2="6" y2="18" />
              <line x1="6" y1="6" x2="18" y2="18" />
            </svg>
          ) : (
            <svg width="20" height="20" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2" strokeLinecap="round" strokeLinejoin="round" aria-hidden="true" focusable="false">
              <line x1="3" y1="6" x2="21" y2="6" />
              <line x1="3" y1="12" x2="21" y2="12" />
              <line x1="3" y1="18" x2="21" y2="18" />
            </svg>
          )}
          <span className="text-2xs uppercase tracking-wider leading-none text-[var(--text-muted)]" aria-hidden="true">
            {currentPageLabel}
          </span>
        </button>

        {/* Navigation - Centered, desktop only */}
        <div className="hidden md:flex flex-1 justify-center max-w-4xl min-w-0">
          <Navigation />
        </div>

        {/* Right Actions */}
         <div className="flex gap-3 items-center">
          <StatusIndicator />
          {loading ? (
            <span className="text-xs text-[var(--text-muted)]">...</span>
          ) : user ? (
            <DropdownMenu.Root>
              <DropdownMenu.Trigger asChild>
                <button
                  type="button"
                  className="flex items-center gap-2 px-2 py-1.5 rounded-[var(--radius-md)] hover:bg-[var(--surface-2)]/60 transition-colors focus-ring"
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

      {menuOpen && (
        <CardShell
          variant="elevated"
          padding="none"
          radius="sm"
          className="md:hidden !rounded-none border-x-0 border-b-0"
        >
          <Navigation mobile onNavigate={closeMenu} />
        </CardShell>
      )}
     </header>
   );
 }
