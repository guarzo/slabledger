/**
 * Navigation Component
 *
 * Minimal text-link navigation with subtle active state styling.
 * Supports mobile (vertical) and desktop (inline) layouts.
 */
import { Link, useLocation } from 'react-router-dom';

interface NavigationProps {
  mobile?: boolean;
  onNavigate?: () => void;
}

export default function Navigation({ mobile, onNavigate }: NavigationProps) {
  const location = useLocation();

  const navItems = [
    { path: '/', label: 'Dashboard', shortLabel: 'Home' },
    { path: '/inventory', label: 'Inventory', shortLabel: 'Inventory' },
    { path: '/campaigns', label: 'Campaigns', shortLabel: 'Campaigns' },
    { path: '/insights', label: 'Insights', shortLabel: 'Insights' },
    { path: '/scan', label: 'Scan', shortLabel: 'Scan' },
    { path: '/reprice', label: 'Reprice', shortLabel: 'Reprice' },
  ];

  const isActive = (path: string) => {
    if (path === '/') return location.pathname === '/';
    return location.pathname === path || location.pathname.startsWith(path + '/');
  };

  const activeClass = 'text-white font-semibold bg-[var(--brand-500)]/15 border border-[var(--brand-500)]/20';
  const inactiveClass = 'text-[var(--text-muted)] font-medium hover:text-[var(--text)] hover:bg-[var(--surface-2)]/10 border border-transparent';

  return (
    <nav
      className={mobile ? 'flex flex-col gap-1 px-4 py-3' : 'flex items-center gap-1'}
      role="navigation"
      aria-label="Main navigation"
    >
      {navItems.map((item) => (
        <Link
          key={item.path}
          to={item.path}
          onClick={mobile ? onNavigate : undefined}
          className={`
            ${mobile ? 'flex items-center px-3.5 py-2.5' : 'relative inline-flex items-center px-3.5 py-2'}
            text-sm rounded-md transition-all duration-200
            ${isActive(item.path) ? activeClass : inactiveClass}
          `}
          title={item.label}
          aria-label={`Navigate to ${item.label}`}
          aria-current={isActive(item.path) ? 'page' : undefined}
        >
          {mobile ? item.label : (
            <>
              <span className="hidden lg:inline leading-none whitespace-nowrap">{item.label}</span>
              <span className="lg:hidden leading-none whitespace-nowrap">{item.shortLabel}</span>
            </>
          )}
        </Link>
      ))}
    </nav>
  );
}
