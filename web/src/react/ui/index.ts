/**
 * UI Component Library
 *
 * Reusable UI components built with Tailwind CSS utilities.
 * All components follow consistent design patterns and support
 * light/dark themes via CSS variables.
 */

// Form Controls
export { default as Button } from './Button';
export type { ButtonProps } from './Button';

export { default as Input } from './Input';
export type { InputProps } from './Input';

export { default as Select } from './Select';
export type { SelectProps } from './Select';

// Layout & Navigation
export { default as Breadcrumb } from './Breadcrumb';
export { default as TabNavigation } from './TabNavigation';
export type { Tab, TabNavigationProps } from './TabNavigation';
export { default as CardShell } from './CardShell';
export type { CardShellProps } from './CardShell';
export { default as EmptyState } from './EmptyState';

// Feedback & Overlays
export { default as ConfirmDialog } from './ConfirmDialog';
export { default as SectionErrorBoundary } from './SectionErrorBoundary';

// Sliders
export { default as DualRangeSlider } from './DualRangeSlider';
export type { DualRangeSliderProps } from './DualRangeSlider';

// Loading
export { default as Skeleton } from './Skeleton';
export type { SkeletonProps } from './Skeleton';

// Interactive
export { default as FavoriteButton } from './FavoriteButton';
export type { FavoriteButtonProps } from './FavoriteButton';

// Price Decision
export { default as PriceDecisionBar } from './PriceDecisionBar';
export type { PriceSource, PriceDecisionBarProps } from './PriceDecisionBar';

// Icons & Dropdowns
export { ExternalLinkIcon } from './ExternalLinkIcon';
export { LinkDropdown } from './LinkDropdown';

// Cards
export { default as CardPriceCard } from './CardPriceCard';
export type { CardPriceData, CardPrices, CardPriceCardProps } from './CardPriceCard';

// Data Display
export { ConfidenceIndicator } from './ConfidenceIndicator';
export { TrendArrow } from './TrendArrow';
export { TrendBadge } from './TrendBadge';

// Stats & Sections
export { default as StatCard } from './StatCard';
export type { StatCardProps } from './StatCard';
export { default as Section } from './Section';
export type { SectionProps } from './Section';
