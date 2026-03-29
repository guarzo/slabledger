import type { PostCardDetail } from '../../../../../types/social';
import { corsUrl } from './corsUrl';

interface FanSpreadProps {
  cards: PostCardDetail[];
}

const FAN_POSITIONS = [
  { left: '5%', top: '8%', width: '42%', rotate: -12, zIndex: 1 },
  { left: '25%', top: '2%', width: '46%', rotate: -2, zIndex: 3 },
  { right: '5%', top: '5%', width: '42%', rotate: 10, zIndex: 2 },
];

export default function FanSpread({ cards }: FanSpreadProps) {
  const visibleCards = cards.slice(0, 3);
  return (
    <div className="absolute top-1/2 left-1/2 -translate-x-1/2 -translate-y-[45%] w-[85%] h-[65%] z-[5]">
      {visibleCards.map((card, i) => {
        const pos = FAN_POSITIONS[i] ?? FAN_POSITIONS[0];
        return (
          <div
            key={card.purchaseId}
            className="absolute rounded-lg overflow-hidden shadow-[0_10px_30px_rgba(0,0,0,0.5)]"
            style={{
              left: pos.left, right: pos.right, top: pos.top,
              width: pos.width, aspectRatio: '0.65',
              transform: `rotate(${pos.rotate}deg)`,
              zIndex: pos.zIndex,
            }}
          >
            {card.frontImageUrl ? (
              <img
                src={corsUrl(card.frontImageUrl)}
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
        );
      })}
    </div>
  );
}
