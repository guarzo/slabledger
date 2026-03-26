/**
 * TabNavigation
 *
 * Reusable tab navigation built on Radix Tabs.
 * Renders only Tabs.List + Tabs.Trigger — the parent must wrap this
 * (and corresponding Tabs.Content panels) inside a Tabs.Root.
 */
import { Tabs } from 'radix-ui';

export interface Tab<T extends string = string> {
  id: T;
  label: string;
}

export interface TabNavigationProps<T extends string = string> {
  tabs: readonly Tab<T>[];
  counts?: Partial<Record<T, number>>;
  ariaLabel?: string;
}

export default function TabNavigation<T extends string = string>({
  tabs,
  counts,
  ariaLabel = 'Navigation tabs',
}: TabNavigationProps<T>) {
  return (
    <Tabs.List
      className="flex gap-1 mb-6 border-b border-[rgba(255,255,255,0.06)] max-w-full overflow-x-auto scrollbar-none"
      aria-label={ariaLabel}
    >
      {tabs.map((t) => {
        const count = counts?.[t.id];
        return (
          <Tabs.Trigger
            key={t.id}
            value={t.id}
            className="px-3 py-2.5 text-sm font-medium transition-colors
                       whitespace-nowrap text-[var(--text-muted)]
                       hover:text-[var(--text)]
                       border-b-2 border-transparent -mb-px
                       data-[state=active]:text-[var(--text)]
                       data-[state=active]:border-[var(--brand-500)]"
          >
            {t.label}
            {count !== undefined ? ` (${count})` : ''}
          </Tabs.Trigger>
        );
      })}
    </Tabs.List>
  );
}
