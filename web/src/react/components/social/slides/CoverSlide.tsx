import type { PostCardDetail, PostType } from '../../../../types/social';
import SlideCanvas from './primitives/SlideCanvas';
import { AccentBar, LogoText } from './primitives/Branding';
import PostTypeBadge from './primitives/PostTypeBadge';
import InfoPanel from './primitives/InfoPanel';
import Flames from './primitives/Flames';
import Sparkles from './primitives/Sparkles';
import TrendLines from './primitives/TrendLines';
import AIBackground from './primitives/AIBackground';
import { getTheme } from './primitives/theme';
import HotDealsCover from './covers/HotDealsCover';
import NewArrivalsCover from './covers/NewArrivalsCover';
import PriceMoversCover from './covers/PriceMoversCover';

interface CoverSlideProps {
  postType: PostType;
  coverTitle: string;
  cardCount: number;
  psa10Count: number;
  cards?: PostCardDetail[];
  backgroundUrls?: string[];
}

function buildSubtitle(postType: PostType, cardCount: number, psa10Count: number, cards?: PostCardDetail[]): string {
  switch (postType) {
    case 'hot_deals': {
      if (!cards?.length) return `${cardCount} cards`;
      let totalDiscount = 0;
      let countWithPrices = 0;
      for (const c of cards) {
        if (c.clValueCents > 0 && c.askingPriceCents > 0 && c.askingPriceCents < c.clValueCents) {
          totalDiscount += 1 - c.askingPriceCents / c.clValueCents;
          countWithPrices++;
        }
      }
      if (countWithPrices === 0) return `${cardCount} cards`;
      const avgDiscount = totalDiscount / countWithPrices;
      return `${cardCount} cards \u00B7 avg ${Math.round(avgDiscount * 100)}% below CL`;
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
  backgroundUrls,
}: CoverSlideProps) {
  const theme = getTheme(postType);
  const subtitle = buildSubtitle(postType, cardCount, psa10Count, cards);

  // Use new post-type-specific covers when cards are available
  if (cards && cards.length > 0) {
    return (
      <SlideCanvas dataSlide="cover">
        {postType === 'hot_deals' && (
          <HotDealsCover postType={postType} coverTitle={coverTitle} cards={cards} backgroundUrl={backgroundUrls?.[0]} />
        )}
        {postType === 'new_arrivals' && (
          <NewArrivalsCover postType={postType} coverTitle={coverTitle} cards={cards} backgroundUrl={backgroundUrls?.[0]} />
        )}
        {postType === 'price_movers' && (
          <PriceMoversCover postType={postType} coverTitle={coverTitle} cards={cards} backgroundUrl={backgroundUrls?.[0]} />
        )}
      </SlideCanvas>
    );
  }

  return (
    <SlideCanvas dataSlide="cover">
      <AIBackground url={backgroundUrls?.[0]} dimming={0.2} />
      <AccentBar gradientBar={theme.gradientBar} />

      <div className="absolute top-[30%] left-1/2 -translate-x-1/2 w-[300px] h-[300px] rounded-full blur-3xl pointer-events-none"
        style={{ background: theme.overlayAccent }} />

      <div className="absolute top-3 left-4 right-4 flex justify-between items-center z-10">
        <LogoText />
        <PostTypeBadge postType={postType} />
      </div>

      {postType === 'hot_deals' && <Flames />}
      {postType === 'new_arrivals' && <Sparkles />}
      {postType === 'price_movers' && <TrendLines />}

      <InfoPanel title={coverTitle} subtitle={subtitle} />
    </SlideCanvas>
  );
}
