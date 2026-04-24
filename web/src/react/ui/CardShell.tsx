import { clsx } from 'clsx';
import type { ComponentPropsWithoutRef, ElementType, ReactNode } from 'react';
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
    >
      {children}
    </Tag>
  );
}

export default CardShell;
