import { clsx } from 'clsx';
import type { ElementType, HTMLAttributes, ReactNode } from 'react';
import styles from './CardShell.module.css';

export type CardVariant = 'default' | 'elevated' | 'glass' | 'premium' | 'ai' | 'data';
export type CardPadding = 'sm' | 'md' | 'lg' | 'none';
export type CardRadius = 'sm' | 'md' | 'lg';

export interface CardShellProps extends Omit<HTMLAttributes<HTMLElement>, 'className'> {
  variant?: CardVariant;
  padding?: CardPadding;
  radius?: CardRadius;
  as?: ElementType;
  interactive?: boolean;
  className?: string;
  children: ReactNode;
}

export function CardShell({
  variant = 'default',
  padding = 'md',
  radius = 'md',
  as: Tag = 'div',
  interactive = false,
  className,
  children,
  ...rest
}: CardShellProps) {
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
