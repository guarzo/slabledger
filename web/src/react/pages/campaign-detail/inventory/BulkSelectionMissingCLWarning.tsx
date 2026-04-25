interface Props {
  missingCLIds: string[];
  selectedCount: number;
  onDeselect: (ids: string[]) => void;
  onHighlight: (ids: string[]) => void;
}

export default function BulkSelectionMissingCLWarning({
  missingCLIds,
  selectedCount,
  onDeselect,
  onHighlight,
}: Props) {
  if (missingCLIds.length === 0) return null;

  return (
    <div className="flex items-center gap-3 text-xs text-[var(--warning)] bg-[var(--warning)]/10 border border-[var(--warning)]/20 rounded-md px-3 py-1.5 mb-2">
      <span>
        ⚠ {missingCLIds.length} of {selectedCount} cards have no CL value
      </span>
      <button
        type="button"
        onClick={() => onHighlight(missingCLIds)}
        className="underline hover:text-[var(--text)]"
      >
        Highlight
      </button>
      <button
        type="button"
        onClick={() => onDeselect(missingCLIds)}
        className="underline hover:text-[var(--text)]"
      >
        Deselect
      </button>
    </div>
  );
}
