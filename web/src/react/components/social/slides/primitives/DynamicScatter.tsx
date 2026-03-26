import type { PostCardDetail } from '../../../../../types/social';

interface DynamicScatterProps {
  cards: PostCardDetail[];
}

interface ScatterPosition {
  left?: string;
  right?: string;
  top: string;
  width: string;
  rotate: number;
  zIndex: number;
  badgePos: Record<string, string>;
}

const SCATTER_POSITIONS: ScatterPosition[] = [
  { left: '0', top: '20%', width: '38%', rotate: -15, zIndex: 1, badgePos: { left: '75%', top: '-5%' } },
  { left: '22%', top: '0', width: '48%', rotate: 3, zIndex: 3, badgePos: { left: '85%', top: '0' } },
  { right: '0', top: '15%', width: '40%', rotate: 12, zIndex: 2, badgePos: { right: '0', top: '-8%' } },
];

export default function DynamicScatter({ cards }: DynamicScatterProps) {
  const visibleCards = cards.slice(0, 3);
  return (
    <div className="absolute top-[48%] left-1/2 -translate-x-1/2 -translate-y-[45%] w-[85%] h-[60%] z-[5]">
      {visibleCards.map((card, i) => {
        const pos = SCATTER_POSITIONS[i] ?? SCATTER_POSITIONS[0];
        const trendPct = card.trend30d !== 0 ? `${card.trend30d > 0 ? '+' : ''}${(card.trend30d * 100).toFixed(0)}%` : null;
        const isUp = card.trend30d > 0;
        return (
          <div key={card.purchaseId} className="absolute" style={{ left: pos.left, right: pos.right, top: pos.top, width: pos.width, zIndex: pos.zIndex }}>
            <div
              className="rounded-lg overflow-hidden"
              style={{
                aspectRatio: '0.65',
                transform: `rotate(${pos.rotate}deg)`,
                boxShadow: `0 ${8 + i * 2}px ${25 + i * 5}px rgba(0,0,0,0.5)`,
              }}
            >
              {card.frontImageUrl ? (
                <img
                  src={card.frontImageUrl}
                  alt={card.cardName}
                  className="w-full h-full object-cover"
                  crossOrigin="anonymous"
                />
              ) : (
                <div className="w-full h-full bg-gray-700 flex items-center justify-center text-white/50 text-xs">
                  {card.cardName}
                </div>
              )}
            </div>
            {trendPct && (
              <div
                className="absolute rounded-lg px-1.5 py-0.5 text-[8px] font-bold text-white z-[4]"
                style={{
                  ...pos.badgePos,
                  background: isUp ? 'rgba(16,185,129,0.9)' : 'rgba(239,68,68,0.9)',
                  boxShadow: isUp ? '0 2px 8px rgba(16,185,129,0.4)' : '0 2px 8px rgba(239,68,68,0.4)',
                }}
              >
                {trendPct}
              </div>
            )}
          </div>
        );
      })}
    </div>
  );
}
