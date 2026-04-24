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
