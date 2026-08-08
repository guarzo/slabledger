import type { FieldChange } from '../../../types/campaigns';
import { targetLanguageOptions } from '../../utils/campaignConstants';

/**
 * The diff field name the mapper emits for the curated-spec-list axis
 * (internal/domain/psacampaign/mapper.go, addList("prepackagedSpecListIds", …)).
 */
export const SPEC_LIST_FIELD = 'prepackagedSpecListIds';

/**
 * The mapper renders list-valued changes with renderStringList: sorted, comma
 * joined, no spaces. Splitting is therefore lossless.
 */
function parseIDList(rendered: string): string[] {
  return rendered.split(',').map(s => s.trim()).filter(Boolean);
}

/**
 * The campaign's own language axis, in words. This is NOT a per-id name: no
 * endpoint exposes the curated spec-list catalog to the browser (the catalog is
 * server-side only, read inside buildResolver), so the ids below are shown raw.
 * Adding GET /api/psa/spec-lists would let this component name each id.
 */
function languageSummary(tokens: string[]): string {
  if (tokens.length === 0) return 'any language (open net)';
  return tokens
    .map(t => targetLanguageOptions.find(o => o.value === t)?.label ?? t)
    .join(', ');
}

function IDChip({ prefix, id, tone }: { prefix: string; id: string; tone: 'danger' | 'success' }) {
  const toneVar = `var(--${tone})`;
  // Deliberately flat (no nested element for `prefix`): Testing Library's
  // getByText only concatenates a node's direct text-node children, so
  // wrapping the prefix in its own colored <span> — as visually tempting as
  // a two-tone chip is — would make "Removed <id>" unmatchable as one string.
  return (
    <span
      className="inline-flex items-baseline gap-1.5 rounded-[var(--radius-sm)] px-2 py-1 font-mono text-2xs font-medium break-all"
      style={{
        backgroundColor: `color-mix(in oklab, ${toneVar} 12%, transparent)`,
        color: toneVar,
      }}
    >
      {prefix} {id}
    </span>
  );
}

/**
 * Renders the curated-spec-list axis as a set diff instead of two comma-joined
 * UUID blobs. A dropped or swapped list is what this exists to make visible:
 * these lists decide what a live campaign spends money on.
 */
export default function SpecListChangeRow({
  change, targetLanguages,
}: {
  change: FieldChange;
  targetLanguages: string[];
}) {
  const before = parseIDList(change.old);
  const after = parseIDList(change.new);
  const beforeSet = new Set(before);
  const afterSet = new Set(after);
  const removed = before.filter(id => !afterSet.has(id));
  const added = after.filter(id => !beforeSet.has(id));
  const keptCount = after.length - added.length;

  return (
    <div className="flex flex-col gap-1.5">
      <div className="flex items-baseline justify-between gap-3">
        <span className="text-[var(--text-subtle)] whitespace-nowrap">Curated spec lists</span>
        <span className="text-[var(--text-muted)] text-right">
          Target languages: {languageSummary(targetLanguages)}
        </span>
      </div>
      {removed.length > 0 && (
        <p className="text-[var(--danger)] font-medium">
          {removed.length === 1
            ? '1 curated list will be REMOVED from this campaign.'
            : `${removed.length} curated lists will be REMOVED from this campaign.`}
        </p>
      )}
      <div className="flex flex-col gap-1">
        {removed.map(id => <IDChip key={id} prefix="Removed" id={id} tone="danger" />)}
        {added.map(id => <IDChip key={id} prefix="Added" id={id} tone="success" />)}
      </div>
      {keptCount > 0 && (
        <span className="text-[var(--text-subtle)]">
          {keptCount === 1 ? '1 unchanged' : `${keptCount} unchanged`}
        </span>
      )}
    </div>
  );
}
