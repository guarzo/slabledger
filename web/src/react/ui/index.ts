/**
 * UI Component Library
 */

// Form Controls
export { default as Button } from './Button';
export type { ButtonProps, ButtonVariant, ButtonSize } from './Button';
export { Segmented } from './Segmented';

export { default as Input } from './Input';
export type { InputProps } from './Input';

export { default as Select } from './Select';
export type { SelectProps } from './Select';

// Layout & Navigation
export { default as Breadcrumb } from './Breadcrumb';
export { default as TabNavigation } from './TabNavigation';
export type { Tab, TabNavigationProps } from './TabNavigation';
export { default as CardShell } from './CardShell';
export type { CardShellProps, CardVariant, CardPadding, CardRadius } from './CardShell';
export { default as EmptyState } from './EmptyState';

// Feedback & Overlays
export { default as ConfirmDialog } from './ConfirmDialog';
export { default as SectionErrorBoundary } from './SectionErrorBoundary';
export { ErrorAlert } from './ErrorAlert';

// Sliders
export { default as DualRangeSlider } from './DualRangeSlider';
export type { DualRangeSliderProps } from './DualRangeSlider';

// Loading
export { default as Skeleton } from './Skeleton';
export type { SkeletonProps } from './Skeleton';

// Price Decision
export { default as PriceDecisionBar } from './PriceDecisionBar';
export type { PriceSource, PriceDecisionBarProps } from './PriceDecisionBar';
export type { PreSelection } from './priceDecisionHelpers';
export { buildPriceSources, preSelectSource } from './priceDecisionHelpers';
export { PricePill } from './PricePill';
export { MarginBadge } from './MarginBadge';

// Icons & Dropdowns
export { ExternalLinkIcon } from './ExternalLinkIcon';
export { LinkDropdown } from './LinkDropdown';

// Grade Display
export { default as GradeBadge } from './GradeBadge';

// Data Display
export { ConfidenceIndicator } from './ConfidenceIndicator';
export { TrendArrow } from './TrendArrow';

// Status & Recommendations
export { StatusPill } from './StatusPill';
export type { StatusTone } from './StatusPill';
export { RecommendationBadge } from './RecommendationBadge';
export type { RecTier, RecSeverity } from './RecommendationBadge';

// Stats & Sections
export { default as StatCard } from './StatCard';
export type { StatCardProps } from './StatCard';
export { default as Section } from './Section';
export type { SectionProps } from './Section';
export { default as SectionEyebrow } from './SectionEyebrow';
export { default as StickyActionBar } from './StickyActionBar';
