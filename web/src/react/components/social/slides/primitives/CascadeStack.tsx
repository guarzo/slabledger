import type { PostCardDetail } from '../../../../../types/social';
import { corsUrl } from './corsUrl';

interface CascadeStackProps {
  cards: PostCardDetail[];
}

const CASCADE_POSITIONS = [
  { left: '0', top: '15px', width: '52%', rotate: -6, zIndex: 1 },
  { left: '18%', top: '0', width: '55%', rotate: 2, zIndex: 2 },
  { right: '0', top: '-5px', width: '50%', rotate: 8, zIndex: 3 },
];

export default function CascadeStack({ cards }: CascadeStackProps) {
  const visibleCards = cards.slice(0, 3);
  return (
    <div className="absolute top-[48%] left-1/2 -translate-x-1/2 -translate-y-[48%] w-[75%] z-[5]"
      style={{ height: '55%' }}>
      {visibleCards.map((card, i) => {
        const pos = CASCADE_POSITIONS[i] ?? CASCADE_POSITIONS[0];
        return (
          <div
            key={card.purchaseId}
            className="absolute rounded-lg overflow-hidden"
            style={{
              left: pos.left, right: pos.right, top: pos.top,
              width: pos.width, aspectRatio: '0.65',
              transform: `rotate(${pos.rotate}deg)`,
              zIndex: pos.zIndex,
              boxShadow: `0 ${8 + i * 4}px ${25 + i * 10}px rgba(0,0,0,${0.4 + i * 0.1})`,
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
