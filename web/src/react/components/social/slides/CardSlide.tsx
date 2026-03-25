import type { PostCardDetail, PostType } from '../../../../types/social';
import logo from '../../../../assets/card-yeti-business-logo.png';

const THEME: Record<PostType, { accent: string; accentBg: string; label: string; gradientBar: string }> = {
  new_arrivals: {
    accent: 'text-indigo-400',
    accentBg: 'bg-indigo-500/15 border-indigo-500/30 text-indigo-400',
    label: 'New Arrivals',
    gradientBar: 'from-transparent via-indigo-500 to-emerald-500',
  },
  price_movers: {
    accent: 'text-amber-400',
    accentBg: 'bg-amber-500/15 border-amber-500/30 text-amber-400',
    label: 'Trending',
    gradientBar: 'from-transparent via-amber-500 to-red-500',
  },
  hot_deals: {
    accent: 'text-emerald-400',
    accentBg: 'bg-emerald-500/15 border-emerald-500/30 text-emerald-400',
    label: 'Deals',
    gradientBar: 'from-transparent via-emerald-500 to-teal-400',
  },
};

interface CardSlideProps {
  card: PostCardDetail;
  postType: PostType;
  slideIndex: number;
  totalSlides: number;
}

export default function CardSlide({ card, postType, slideIndex, totalSlides }: CardSlideProps) {
  const theme = THEME[postType] ?? THEME.new_arrivals;
  const marketPrice = card.medianCents > 0 ? `$${(card.medianCents / 100).toFixed(0)}` : null;
  const gradeDisplay = `${card.grader || 'PSA'} ${card.gradeValue}`;
  const buyPrice = card.buyCostCents > 0 ? `$${(card.buyCostCents / 100).toFixed(0)}` : null;

  return (
    <div
      className="w-full h-full bg-gradient-to-br from-[#0a0e1a] via-[#111827] to-[#1a1f35] flex flex-col p-6 text-white relative overflow-hidden"
      data-slide="card"
    >
      {/* Accent bar */}
      <div className={`absolute top-0 left-0 right-0 h-[3px] bg-gradient-to-r ${theme.gradientBar}`} />

      {/* Header: logo + slide counter */}
      <div className="flex items-center justify-between mb-4 mt-1">
        <img src={logo} alt="Card Yeti" className="w-[80px]" />
        <span className="text-xs text-white/30">{slideIndex + 1} / {totalSlides}</span>
      </div>

      {/* Card image */}
      <div className="flex-1 flex items-center justify-center w-full mb-4 min-h-0 max-h-[60%]">
        {card.frontImageUrl ? (
          <img
            src={card.frontImageUrl}
            alt={card.cardName}
            className="max-h-full max-w-full object-contain rounded-lg shadow-2xl drop-shadow-[0_0_30px_rgba(99,102,241,0.15)]"
            crossOrigin="anonymous"
          />
        ) : (
          <div className="w-48 h-64 bg-white/5 border border-white/10 rounded-lg flex flex-col items-center justify-center p-4">
            <span className="text-lg font-bold text-center text-white/80 mb-2">{gradeDisplay}</span>
            <span className="text-sm text-center text-white/50">{card.cardName}</span>
          </div>
        )}
      </div>

      {/* Info panel */}
      <div className="bg-white/[0.04] border border-white/[0.08] rounded-xl p-4 space-y-2">
        {/* Row 1: Name + grade badge */}
        <div className="flex items-center justify-between gap-2">
          <h3 className="text-base font-bold truncate flex-1">{card.cardName}</h3>
          <span className={`shrink-0 bg-gradient-to-r ${theme.gradientBar} rounded-md px-3 py-1 text-sm font-bold`}>
            {gradeDisplay}
          </span>
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
              {marketPrice && (
                <span className="text-lg font-bold text-emerald-400">{marketPrice}</span>
              )}
              <span className="text-xs text-white/40">Cert #{card.certNumber}</span>
            </>
          )}

          {postType === 'price_movers' && (
            <>
              <div className="flex items-center gap-2">
                {marketPrice && (
                  <span className="text-lg font-bold text-white">{marketPrice}</span>
                )}
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
                {marketPrice && (
                  <span className="text-lg font-bold text-emerald-400">{marketPrice}</span>
                )}
                {buyPrice && (
                  <span className="text-sm text-white/40 line-through">{buyPrice}</span>
                )}
              </div>
              <span className="text-xs text-white/40">Cert #{card.certNumber}</span>
            </>
          )}
        </div>
      </div>

      {/* Footer */}
      <div className="mt-3 text-center">
        <span className="text-xs text-white/40">cardyeti.com</span>
      </div>
    </div>
  );
}
