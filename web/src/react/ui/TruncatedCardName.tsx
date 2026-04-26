interface TruncatedCardNameProps {
  /** Full card name. Shown as native tooltip on hover. */
  name: string;
  /** Extra Tailwind classes to merge in. */
  className?: string;
}

/**
 * Renders a card name truncated to 2 lines with ellipsis. The full name is
 * available on hover via the native `title` attribute. Designed for use in
 * dense table cells (Inventory, Reprice) where horizontal space is constrained.
 */
export default function TruncatedCardName({ name, className }: TruncatedCardNameProps) {
  return (
    <span
      title={name}
      className={`line-clamp-2 break-words ${className ?? ''}`.trim()}
    >
      {name}
    </span>
  );
}
