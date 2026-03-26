import type { PostCardDetail, PostType } from '../../../../../types/social';
import GradeBadge from './GradeBadge';

interface CardInfoPanelProps {
  card: PostCardDetail;
  postType: PostType;
}

export default function CardInfoPanel({ card, postType }: CardInfoPanelProps) {
  const askingPrice = card.askingPriceCents > 0 ? `$${(card.askingPriceCents / 100).toFixed(0)}` : null;
  const clPrice = card.clValueCents > 0 ? `$${(card.clValueCents / 100).toFixed(0)}` : null;
  const showBothPrices = askingPrice && clPrice && card.askingPriceCents <= card.clValueCents;

  return (
    <div className="bg-white/[0.04] border border-white/[0.08] rounded-xl p-4 space-y-2">
      {/* Row 1: Name + grade badge */}
      <div className="flex items-center justify-between gap-2">
        <h3 className="text-base font-bold truncate flex-1">{card.cardName}</h3>
        <GradeBadge grader={card.grader} grade={card.gradeValue} postType={postType} />
      </div>

      {/* Row 2: Set + card number */}
      {card.setName && (
        <p className="text-xs text-white/50">
          {card.setName}{card.cardNumber ? ` \u00B7 #${card.cardNumber}` : ''}
        </p>
      )}

      {/* Row 3: Post-type-specific data */}
      <div className="flex items-center justify-between pt-1">
        {postType === 'new_arrivals' && (
          <>
            {askingPrice && <span className="text-lg font-bold text-emerald-400">{askingPrice}</span>}
            <span className="text-xs text-white/40">Cert #{card.certNumber}</span>
          </>
        )}
        {postType === 'price_movers' && (
          <>
            <div className="flex items-center gap-2">
              {askingPrice && <span className="text-lg font-bold text-white">{askingPrice}</span>}
              {card.trend30d !== 0 && (
                <span className={`text-sm font-medium ${card.trend30d > 0 ? 'text-emerald-400' : 'text-red-400'}`}>
                  {card.trend30d > 0 ? '+' : ''}{(card.trend30d * 100).toFixed(1)}%
                </span>
              )}
            </div>
            <span className="text-xs text-white/40">Cert #{card.certNumber}</span>
          </>
        )}
        {postType === 'hot_deals' && (
          <>
            <div className="flex items-center gap-2">
              {askingPrice && <span className="text-lg font-bold text-emerald-400">{askingPrice}</span>}
              {showBothPrices && <span className="text-sm text-white/40">CL {clPrice}</span>}
            </div>
            <span className="text-xs text-white/40">Cert #{card.certNumber}</span>
          </>
        )}
      </div>
    </div>
  );
}
