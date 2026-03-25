import { defaultEbayUrl, defaultAltUrl, defaultCardLadderUrl, gradeToGradeKey } from '../../../utils/marketplaceUrls';
import { LinkDropdown } from '../../../ui/LinkDropdown';

interface MarketplaceLinksProps {
  cardName: string;
  setName: string;
  cardNumber: string;
  gradeValue: number;
  variant: 'expanded' | 'inline';
  stopPropagation?: boolean;
}

export default function MarketplaceLinks({ cardName, setName, cardNumber, gradeValue, variant, stopPropagation }: MarketplaceLinksProps) {
  const card = { name: cardName, setName, number: cardNumber };
  const grade = gradeToGradeKey(gradeValue);

  if (variant === 'expanded') {
    return (
      <div className="flex items-center gap-2 ml-4 shrink-0">
        <a
          href={defaultEbayUrl(card, grade)}
          target="_blank"
          rel="noopener noreferrer"
          className="text-xs px-2 py-0.5 rounded bg-[var(--surface-2)]/60 text-[var(--text-muted)] hover:text-[var(--brand-400)] hover:bg-[var(--surface-2)] transition-colors"
        >eBay</a>
        <a
          href={defaultAltUrl(card, grade)}
          target="_blank"
          rel="noopener noreferrer"
          className="text-xs px-2 py-0.5 rounded bg-[var(--surface-2)]/60 text-[var(--text-muted)] hover:text-[var(--brand-400)] hover:bg-[var(--surface-2)] transition-colors"
        >Alt</a>
        <a
          href={defaultCardLadderUrl(card, grade)}
          target="_blank"
          rel="noopener noreferrer"
          className="text-xs px-2 py-0.5 rounded bg-[var(--surface-2)]/60 text-[var(--text-muted)] hover:text-[var(--brand-400)] hover:bg-[var(--surface-2)] transition-colors"
        >CardLadder</a>
      </div>
    );
  }

  const links = [
    { label: 'eBay', href: defaultEbayUrl(card, grade) },
    { label: 'Alt', href: defaultAltUrl(card, grade) },
    { label: 'CardLadder', href: defaultCardLadderUrl(card, grade) },
  ];

  return <LinkDropdown links={links} stopPropagation={stopPropagation} />;
}
