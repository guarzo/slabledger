import type { PostCardDetail, PostType } from '../../../../types/social';
import SlideCanvas from './primitives/SlideCanvas';
import { AccentBar, LogoText } from './primitives/Branding';
import PostTypeBadge from './primitives/PostTypeBadge';
import InfoPanel from './primitives/InfoPanel';
import FanSpread from './primitives/FanSpread';
import CascadeStack from './primitives/CascadeStack';
import DynamicScatter from './primitives/DynamicScatter';
import Flames from './primitives/Flames';
import Sparkles from './primitives/Sparkles';
import TrendLines from './primitives/TrendLines';
import { getTheme } from './primitives/theme';

interface CoverSlideProps {
  postType: PostType;
  coverTitle: string;
  cardCount: number;
  createdAt: string;
  psa10Count: number;
  totalValueCents: number;
  cards?: PostCardDetail[];
}

function buildSubtitle(postType: PostType, cardCount: number, psa10Count: number, cards?: PostCardDetail[]): string {
  switch (postType) {
    case 'hot_deals': {
      if (!cards?.length) return `${cardCount} cards`;
      let totalDiscount = 0;
      let countWithPrices = 0;
      for (const c of cards) {
        if (c.medianCents > 0 && c.buyCostCents > 0) {
          totalDiscount += 1 - c.buyCostCents / c.medianCents;
          countWithPrices++;
        }
      }
      const avgDiscount = countWithPrices > 0 ? totalDiscount / countWithPrices : 0;
      return `${cardCount} cards \u00B7 avg ${Math.round(avgDiscount * 100)}% below market`;
    }
    case 'new_arrivals':
      return `${cardCount} cards \u00B7 ${psa10Count}\u00D7 PSA 10`;
    case 'price_movers': {
      if (!cards?.length) return `${cardCount} cards`;
      const up = cards.filter(c => c.trend30d > 0).length;
      const down = cards.filter(c => c.trend30d < 0).length;
      return `${cardCount} cards \u00B7 ${up} up \u00B7 ${down} down`;
    }
    default:
      return `${cardCount} cards`;
  }
}

export default function CoverSlide({
  postType,
  coverTitle,
  cardCount,
  psa10Count,
  cards,
}: CoverSlideProps) {
  const theme = getTheme(postType);
  const subtitle = buildSubtitle(postType, cardCount, psa10Count, cards);

  return (
    <SlideCanvas dataSlide="cover">
      <AccentBar gradientBar={theme.gradientBar} />

      {/* Radial glow */}
      <div className="absolute top-[30%] left-1/2 -translate-x-1/2 w-[300px] h-[300px] rounded-full blur-3xl pointer-events-none"
        style={{ background: theme.overlayAccent }} />

      {/* Header */}
      <div className="absolute top-3 left-4 right-4 flex justify-between items-center z-10">
        <LogoText />
        <PostTypeBadge postType={postType} />
      </div>

      {/* Overlays */}
      {postType === 'hot_deals' && <Flames />}
      {postType === 'new_arrivals' && <Sparkles />}
      {postType === 'price_movers' && <TrendLines />}

      {/* Card layout */}
      {cards && cards.length > 0 && (
        <>
          {postType === 'hot_deals' && <FanSpread cards={cards} />}
          {postType === 'new_arrivals' && <CascadeStack cards={cards} />}
          {postType === 'price_movers' && <DynamicScatter cards={cards} />}
        </>
      )}

      {/* Bottom info */}
      <InfoPanel title={coverTitle} subtitle={subtitle} />
    </SlideCanvas>
  );
}
