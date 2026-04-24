import { clsx } from 'clsx';
import type {
  ComponentPropsWithoutRef,
  ElementType,
  KeyboardEvent,
  MouseEvent,
  ReactNode,
} from 'react';
import styles from './CardShell.module.css';

export type CardVariant = 'default' | 'elevated' | 'glass' | 'premium' | 'ai' | 'data';
export type CardPadding = 'sm' | 'md' | 'lg' | 'none';
export type CardRadius = 'sm' | 'md' | 'lg';

type CardShellOwnProps<T extends ElementType> = {
  variant?: CardVariant;
  padding?: CardPadding;
  radius?: CardRadius;
  as?: T;
  interactive?: boolean;
  className?: string;
  children: ReactNode;
};

export type CardShellProps<T extends ElementType = 'div'> = CardShellOwnProps<T> &
  Omit<ComponentPropsWithoutRef<T>, keyof CardShellOwnProps<T>>;

/**
 * CardShell — typed card primitive with six variants.
 *
 * ## Clickable contract
 *
 * If you pass `onClick`, you MUST pick one of these to keep the card
 * keyboard-operable (WCAG 2.1 AA requirement):
 *
 * - `as="button"` (or `as="a"` with `href`) — native element, native semantics.
 * - `interactive` — CardShell adds `tabIndex`, `role="button"`, and
 *   Enter/Space keyboard activation automatically for the div fallback.
 * - Explicit `role` + `tabIndex` — caller takes full responsibility.
 *
 * A clickable div without any of these is a silent a11y footgun: mouse
 * works, keyboard doesn't. In development, this component warns when that
 * pattern is detected.
 */
export function CardShell<T extends ElementType = 'div'>({
  variant = 'default',
  padding = 'md',
  radius = 'md',
  as,
  interactive = false,
  className,
  children,
  ...rest
}: CardShellProps<T>) {
  const Tag = (as ?? 'div') as ElementType;

  if (import.meta.env.DEV) {
    warnOnBrokenClickableContract(Tag, interactive, rest as Record<string, unknown>);
  }

  // Interactive divs aren't keyboard-operable by default. When a caller opts
  // into `interactive` without overriding `as`, shim in tabIndex, role, and
  // Enter/Space keyboard activation. Real buttons/anchors (`as="button"` /
  // `as="a"`) get native semantics and skip this shim entirely.
  const needsA11yShim = interactive && Tag === 'div';
  const a11yProps = needsA11yShim ? buildA11yShim(rest as Record<string, unknown>) : undefined;

  return (
    <Tag
      className={clsx(
        styles.card,
        styles[`v-${variant}`],
        styles[`p-${padding}`],
        styles[`r-${radius}`],
        interactive && styles.interactive,
        className,
      )}
      {...rest}
      {...a11yProps}
    >
      {children}
    </Tag>
  );
}

const warnedContractViolations = new WeakSet<object>();

function warnOnBrokenClickableContract(
  Tag: ElementType,
  interactive: boolean,
  rest: Record<string, unknown>,
): void {
  const onClick = rest.onClick;
  if (!onClick || typeof onClick !== 'function') return;

  // Exempt when: caller picked a natively interactive element, opted in to
  // the shim via `interactive`, or supplied role/tabIndex themselves.
  const isNativeInteractive = Tag === 'button' || Tag === 'a';
  const hasRoleOrTabIndex = rest.role !== undefined || rest.tabIndex !== undefined;
  if (isNativeInteractive || interactive || hasRoleOrTabIndex) return;

  // Once per handler identity to avoid log spam on re-renders.
  if (warnedContractViolations.has(onClick)) return;
  warnedContractViolations.add(onClick);

  // eslint-disable-next-line no-console
  console.warn(
    'CardShell: onClick was passed on a non-interactive element. ' +
      'Add `as="button"`, `interactive`, or explicit `role`/`tabIndex` — ' +
      'otherwise keyboard users cannot activate the card.',
  );
}

function buildA11yShim(rest: Record<string, unknown>): Record<string, unknown> {
  const userOnClick = rest.onClick as ((e: MouseEvent<HTMLElement>) => void) | undefined;
  const userOnKeyDown = rest.onKeyDown as ((e: KeyboardEvent<HTMLElement>) => void) | undefined;
  return {
    tabIndex: rest.tabIndex ?? 0,
    role: rest.role ?? 'button',
    onKeyDown: (e: KeyboardEvent<HTMLElement>) => {
      userOnKeyDown?.(e);
      if (!e.defaultPrevented && (e.key === 'Enter' || e.key === ' ') && userOnClick) {
        e.preventDefault();
        userOnClick(e as unknown as MouseEvent<HTMLElement>);
      }
    },
  };
}

export default CardShell;
