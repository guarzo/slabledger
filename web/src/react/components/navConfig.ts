/**
 * Single source of truth for navigation.
 *
 * Consumed by Header (top chrome), MobileNav (hamburger drawer),
 * and CommandPalette (⌘K typeable shortcut).
 */

export type NavZone = 'home' | 'browse' | 'do' | 'hidden';
export type NavGroup = 'reports' | 'scan';

export interface NavItem {
  path: string;
  label: string;
  shortLabel: string;
  zone: NavZone;
  group?: NavGroup;
  description?: string;
  primary?: boolean;
  adminOnly?: boolean;
}

export const NAV_ITEMS: NavItem[] = [
  { path: '/', label: 'Dashboard', shortLabel: 'Home', zone: 'home' },

  { path: '/campaigns', label: 'Campaigns', shortLabel: 'Campaigns', zone: 'browse' },
  { path: '/inventory', label: 'Inventory', shortLabel: 'Inventory', zone: 'browse' },

  {
    path: '/insights',
    label: 'Insights',
    shortLabel: 'Insights',
    zone: 'browse',
    group: 'reports',
    description: 'AI signals',
  },
  {
    path: '/opportunities/psa-exchange',
    label: 'Opportunities',
    shortLabel: 'Opps',
    zone: 'browse',
    group: 'reports',
    description: 'PSA-Exchange listings',
  },

  {
    path: '/scan',
    label: 'Scan',
    shortLabel: 'Scan',
    zone: 'do',
    group: 'scan',
    description: 'Cert lookup, intake',
    primary: true,
  },
  {
    path: '/reprice',
    label: 'Reprice',
    shortLabel: 'Reprice',
    zone: 'do',
    group: 'scan',
    description: 'Suggested prices',
  },
  {
    path: '/sell-sheet',
    label: 'Sell Sheet',
    shortLabel: 'Sheet',
    zone: 'do',
    group: 'scan',
    description: 'Print or share',
  },

  {
    path: '/invoices',
    label: 'Invoices',
    shortLabel: 'Invoices',
    zone: 'hidden',
    description: 'Capital, due dates',
  },
  {
    path: '/admin',
    label: 'Admin',
    shortLabel: 'Admin',
    zone: 'hidden',
    description: 'API health, users',
    adminOnly: true,
  },
];

export function navItemsForZone(zone: NavZone): NavItem[] {
  return NAV_ITEMS.filter((item) => item.zone === zone && !item.group);
}

export function navItemsForGroup(group: NavGroup): NavItem[] {
  return NAV_ITEMS.filter((item) => item.group === group);
}

export function primaryItemForGroup(group: NavGroup): NavItem | undefined {
  return navItemsForGroup(group).find((item) => item.primary) ?? navItemsForGroup(group)[0];
}

export function paletteItems(isAdmin: boolean): NavItem[] {
  return NAV_ITEMS.filter((item) => !item.adminOnly || isAdmin);
}

export function isRouteActive(itemPath: string, currentPath: string): boolean {
  if (itemPath === '/') return currentPath === '/';
  return currentPath === itemPath || currentPath.startsWith(itemPath + '/');
}

export function pageLabelForPath(currentPath: string): string {
  const match = NAV_ITEMS
    .filter((item) => isRouteActive(item.path, currentPath))
    .sort((a, b) => b.path.length - a.path.length)[0];
  return match?.label ?? 'Menu';
}
