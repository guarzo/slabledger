import type { PostCardDetail, PostType } from '../../../../types/social';
import SlideCanvas from './primitives/SlideCanvas';
import { AccentBar, Logo, SlideCounter } from './primitives/Branding';
import CardInfoPanel from './primitives/CardInfoPanel';
import SlabAccent from './primitives/SlabAccent';
import CardArtHero from './primitives/CardArtHero';
import Flames from './primitives/Flames';
import Sparkles from './primitives/Sparkles';
import TrendLines from './primitives/TrendLines';
import AIBackground from './primitives/AIBackground';
import { getTheme } from './primitives/theme';

interface CardSlideProps {
  card: PostCardDetail;
  postType: PostType;
  slideIndex: number;
  totalSlides: number;
  backgroundUrl?: string;
}

export default function CardSlide({ card, postType, slideIndex, totalSlides, backgroundUrl }: CardSlideProps) {
  const theme = getTheme(postType);
  const isHero = postType === 'new_arrivals';

  return (
    <SlideCanvas dataSlide="card">
      <AIBackground url={backgroundUrl} dimming={isHero ? 0.3 : 0.4} />
      <div className="flex flex-col p-6 h-full">
        <AccentBar gradientBar={theme.gradientBar} />

        {/* Header: logo + slide counter */}
        <div className="flex items-center justify-between mb-4 mt-1 relative z-10">
          <Logo size="sm" />
          <SlideCounter current={slideIndex + 1} total={totalSlides} />
        </div>

        {/* Subtle overlay behind card */}
        {postType === 'hot_deals' && <Flames intensity="subtle" />}
        {postType === 'new_arrivals' && <Sparkles />}
        {postType === 'price_movers' && <TrendLines intensity="subtle" />}

        {/* Card layout — hero for new arrivals, slab accent for others */}
        {isHero ? (
          <CardArtHero
            imageUrl={card.frontImageUrl}
            cardName={card.cardName}
            grader={card.grader}
            grade={card.gradeValue}
            backgroundUrl={backgroundUrl}
          />
        ) : (
          <SlabAccent
            imageUrl={card.frontImageUrl}
            cardName={card.cardName}
            postType={postType}
          />
        )}

        {/* Info panel */}
        <CardInfoPanel card={card} postType={postType} />

        {/* Footer */}
        <div className="mt-3 text-center">
          <span className="text-xs text-white/40">cardyeti.com</span>
        </div>
      </div>
    </SlideCanvas>
  );
}
