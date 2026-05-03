/**
 * Header Component
 *
 * Sticky header with three-zone top chrome (home / browse / do) plus
 * a global ⌘K command palette and the user menu.
 */
import { useCallback, useEffect, useState } from 'react';
import { Link, useLocation } from 'react-router-dom';
import { DropdownMenu } from 'radix-ui';
import StatusIndicator from './StatusIndicator';
import CommandPalette from './CommandPalette';
import {
  NAV_ITEMS,
  isRouteActive,
  navItemsForGroup,
  navItemsForZone,
  pageLabelForPath,
  primaryItemForGroup,
  type NavItem,
} from './navConfig';
import { useAuth } from '../contexts/AuthContext';
import CardShell from '../ui/CardShell';
import logoSrc from '../../assets/logo.png';

const browseDirectClass =
  'relative inline-flex items-center px-3.5 py-2 text-sm rounded-md transition-colors duration-200 border focus-ring whitespace-nowrap';
const browseActiveClass = 'text-white font-semibold bg-[var(--brand-500)]/15 border-[var(--brand-500)]/20';
const browseInactiveClass = 'text-[var(--text-muted)] font-medium hover:text-[var(--text)] hover:bg-[var(--surface-2)]/10 border-transparent';

function isMac(): boolean {
  if (typeof navigator === 'undefined') return false;
  return /Mac|iPod|iPhone|iPad/.test(navigator.platform);
}

function UserInitial({ name }: { name: string }) {
  const initial = name.charAt(0).toUpperCase();
  return (
    <div className="w-8 h-8 rounded-full bg-[var(--surface-2)] flex items-center justify-center text-[var(--text)] text-sm font-bold">
      {initial}
    </div>
  );
}

function ChevronDown({ className = '' }: { className?: string }) {
  return (
    <svg width="12" height="12" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2" strokeLinecap="round" strokeLinejoin="round" aria-hidden="true" focusable="false" className={className}>
      <path d="m6 9 6 6 6-6" />
    </svg>
  );
}

function BrowseDirect({ item, currentPath }: { item: NavItem; currentPath: string }) {
  const active = isRouteActive(item.path, currentPath);
  return (
    <Link
      to={item.path}
      className={`${browseDirectClass} ${active ? browseActiveClass : browseInactiveClass}`}
      aria-current={active ? 'page' : undefined}
    >
      {item.label}
    </Link>
  );
}

function ReportsDropdown({ currentPath }: { currentPath: string }) {
  const items = navItemsForGroup('reports');
  const anyActive = items.some((it) => isRouteActive(it.path, currentPath));
  return (
    <DropdownMenu.Root>
      <DropdownMenu.Trigger asChild>
        <button
          type="button"
          className={`${browseDirectClass} ${anyActive ? browseActiveClass : browseInactiveClass} gap-1.5`}
          aria-label="Reports menu"
        >
          Reports
          <ChevronDown />
        </button>
      </DropdownMenu.Trigger>
      <DropdownMenu.Portal>
        <DropdownMenu.Content
          align="start"
          sideOffset={6}
          className="min-w-[220px] py-1 bg-[var(--surface-1)] border border-[var(--surface-2)] rounded-[var(--radius-md)] shadow-[var(--shadow-2)] z-50 data-[state=open]:animate-[fadeIn_150ms_ease-out]"
        >
          {items.map((item) => (
            <DropdownMenu.Item key={item.path} asChild>
              <Link
                to={item.path}
                className="flex flex-col gap-0.5 px-3 py-2 text-sm text-[var(--text)] hover:bg-[var(--surface-2)]/60 data-[highlighted]:bg-[var(--surface-2)]/60 outline-none cursor-default focus-ring rounded-sm mx-1"
              >
                <span className="font-medium">{item.label}</span>
                {item.description && (
                  <span className="text-2xs text-[var(--text-subtle)]">{item.description}</span>
                )}
              </Link>
            </DropdownMenu.Item>
          ))}
        </DropdownMenu.Content>
      </DropdownMenu.Portal>
    </DropdownMenu.Root>
  );
}

function ScanSplitButton({ currentPath }: { currentPath: string }) {
  const items = navItemsForGroup('scan');
  const primary = primaryItemForGroup('scan');
  const anyActive = items.some((it) => isRouteActive(it.path, currentPath));
  if (!primary) return null;

  const activeFill =
    'text-white font-semibold bg-[var(--brand-500)]/20 border-[var(--brand-500)]/40 shadow-[var(--shadow-1)]';
  const restingFill =
    'text-[var(--text)] font-medium bg-[var(--surface-2)]/60 border-[var(--surface-3)] hover:bg-[var(--surface-2)]';
  const sharedSegment = 'inline-flex items-center px-3 py-2 text-sm border focus-ring whitespace-nowrap transition-colors';

  return (
    <div className="inline-flex rounded-md overflow-hidden">
      <Link
        to={primary.path}
        aria-current={isRouteActive(primary.path, currentPath) ? 'page' : undefined}
        className={`${sharedSegment} ${anyActive ? activeFill : restingFill} border-r-0 rounded-l-md`}
      >
        {primary.label}
      </Link>
      <DropdownMenu.Root>
        <DropdownMenu.Trigger asChild>
          <button
            type="button"
            aria-label="More do-zone actions"
            className={`${sharedSegment} ${anyActive ? activeFill : restingFill} rounded-r-md px-2`}
          >
            <ChevronDown />
          </button>
        </DropdownMenu.Trigger>
        <DropdownMenu.Portal>
          <DropdownMenu.Content
            align="end"
            sideOffset={6}
            className="min-w-[220px] py-1 bg-[var(--surface-1)] border border-[var(--surface-2)] rounded-[var(--radius-md)] shadow-[var(--shadow-2)] z-50 data-[state=open]:animate-[fadeIn_150ms_ease-out]"
          >
            {items.map((item) => (
              <DropdownMenu.Item key={item.path} asChild>
                <Link
                  to={item.path}
                  className="flex flex-col gap-0.5 px-3 py-2 text-sm text-[var(--text)] hover:bg-[var(--surface-2)]/60 data-[highlighted]:bg-[var(--surface-2)]/60 outline-none cursor-default focus-ring rounded-sm mx-1"
                >
                  <span className="font-medium">{item.label}</span>
                  {item.description && (
                    <span className="text-2xs text-[var(--text-subtle)]">{item.description}</span>
                  )}
                </Link>
              </DropdownMenu.Item>
            ))}
          </DropdownMenu.Content>
        </DropdownMenu.Portal>
      </DropdownMenu.Root>
    </div>
  );
}

function PaletteButton({ onOpen }: { onOpen: () => void }) {
  const shortcut = isMac() ? '⌘K' : 'Ctrl K';
  return (
    <button
      type="button"
      onClick={onOpen}
      aria-label="Open command palette"
      className="hidden md:inline-flex items-center gap-2 px-2.5 py-1.5 text-xs text-[var(--text-muted)] bg-[var(--surface-2)]/40 hover:bg-[var(--surface-2)]/70 hover:text-[var(--text)] border border-[var(--surface-3)] rounded-md transition-colors focus-ring"
    >
      <svg width="14" height="14" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2" strokeLinecap="round" strokeLinejoin="round" aria-hidden="true">
        <circle cx="11" cy="11" r="7" />
        <path d="m20 20-3.5-3.5" />
      </svg>
      <span className="font-mono uppercase tracking-wider text-2xs">{shortcut}</span>
    </button>
  );
}

function MobileDrawer({ currentPath, onNavigate, isAdmin }: { currentPath: string; onNavigate: () => void; isAdmin: boolean }) {
  const sections: { key: string; heading: string; items: NavItem[] }[] = [
    { key: 'home', heading: 'Home', items: navItemsForZone('home') },
    {
      key: 'browse',
      heading: 'Browse',
      items: [...navItemsForZone('browse'), ...navItemsForGroup('reports')],
    },
    { key: 'do', heading: 'Do', items: navItemsForGroup('scan') },
  ];
  const hidden = NAV_ITEMS.filter((it) => it.zone === 'hidden' && (!it.adminOnly || isAdmin));
  if (hidden.length > 0) {
    sections.push({ key: 'more', heading: 'More', items: hidden });
  }

  return (
    <nav className="flex flex-col gap-1 px-4 py-3" role="navigation" aria-label="Main navigation">
      {sections.map((section) => (
        <div key={section.key} className="mb-2 last:mb-0">
          <div className="text-2xs uppercase tracking-wider text-[var(--text-subtle)] font-semibold px-3 py-1">
            {section.heading}
          </div>
          {section.items.map((item) => {
            const active = isRouteActive(item.path, currentPath);
            return (
              <Link
                key={item.path}
                to={item.path}
                onClick={onNavigate}
                className={`flex items-center px-3.5 py-2.5 text-sm rounded-md transition-colors duration-200 border focus-ring ${
                  active ? browseActiveClass : browseInactiveClass
                }`}
                aria-current={active ? 'page' : undefined}
              >
                {item.label}
                {item.description && (
                  <span className="ml-3 text-2xs text-[var(--text-subtle)]">{item.description}</span>
                )}
              </Link>
            );
          })}
        </div>
      ))}
    </nav>
  );
}

export default function Header() {
  const { user, loading, logout } = useAuth();
  const [isScrolled, setIsScrolled] = useState(false);
  const [avatarError, setAvatarError] = useState(false);
  const [menuOpen, setMenuOpen] = useState(false);
  const [paletteOpen, setPaletteOpen] = useState(false);
  const closeMenu = useCallback(() => setMenuOpen(false), []);
  const location = useLocation();
  const currentPageLabel = pageLabelForPath(location.pathname);

  useEffect(() => {
    setMenuOpen(false);
  }, [location.pathname]);

  useEffect(() => {
    const onKey = (e: KeyboardEvent) => {
      if (e.key === 'Escape') setMenuOpen(false);
      const isPaletteShortcut = (e.metaKey || e.ctrlKey) && (e.key === 'k' || e.key === 'K');
      if (!isPaletteShortcut) return;
      // Don't hijack ⌘K when the user is typing in an input, textarea, or
      // contenteditable surface — the palette is a global navigation aid,
      // not an editor escape.
      const target = e.target as HTMLElement | null;
      if (target) {
        const tag = target.tagName;
        if (tag === 'INPUT' || tag === 'TEXTAREA' || target.isContentEditable) return;
      }
      e.preventDefault();
      // Open-only — match PaletteButton's onOpen behavior. Esc and outside-
      // click already handle close, so the shortcut never needs to toggle.
      setPaletteOpen(true);
    };
    document.addEventListener('keydown', onKey);
    return () => document.removeEventListener('keydown', onKey);
  }, []);

  useEffect(() => {
    setAvatarError(false);
  }, [user]);

  useEffect(() => {
    const handleScroll = () => setIsScrolled(window.scrollY > 10);
    handleScroll();
    window.addEventListener('scroll', handleScroll);
    return () => window.removeEventListener('scroll', handleScroll);
  }, []);

  const homeItem = navItemsForZone('home')[0];
  const browseItems = navItemsForZone('browse');

  return (
    <header
      className={`
        sticky top-0 z-50
        bg-[var(--surface-1)]/80
        border-b border-[rgba(255,255,255,0.06)]
        backdrop-blur-xl
        transition-shadow duration-300
        ${isScrolled ? 'shadow-md' : 'shadow-sm'}
      `}
      role="banner"
    >
      <div className="flex items-center justify-between gap-4 px-6 py-2.5 max-w-[1920px] mx-auto">
        {/* Logo */}
        <Link to="/" className="flex items-center gap-2 group relative">
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

        {/* Hamburger — mobile only */}
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

        {/* Three-zone navigation — desktop only */}
        <nav
          className="hidden md:flex flex-1 items-center justify-center gap-2 min-w-0"
          role="navigation"
          aria-label="Main navigation"
        >
          {homeItem && <BrowseDirect item={homeItem} currentPath={location.pathname} />}
          <span className="w-px h-5 bg-[rgba(255,255,255,0.08)] mx-1" aria-hidden="true" />
          {browseItems.map((item) => (
            <BrowseDirect key={item.path} item={item} currentPath={location.pathname} />
          ))}
          <ReportsDropdown currentPath={location.pathname} />
          <span className="w-px h-5 bg-[rgba(255,255,255,0.08)] mx-1" aria-hidden="true" />
          <ScanSplitButton currentPath={location.pathname} />
        </nav>

        {/* Right cluster */}
        <div className="flex gap-2 items-center">
          <PaletteButton onOpen={() => setPaletteOpen(true)} />
          <StatusIndicator />
          {loading ? (
            <span className="text-xs text-[var(--text-muted)]">…</span>
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
                  <ChevronDown className="text-[var(--text-muted)]" />
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
          <MobileDrawer
            currentPath={location.pathname}
            onNavigate={closeMenu}
            isAdmin={!!user?.is_admin}
          />
        </CardShell>
      )}

      <CommandPalette open={paletteOpen} onOpenChange={setPaletteOpen} />
    </header>
  );
}
