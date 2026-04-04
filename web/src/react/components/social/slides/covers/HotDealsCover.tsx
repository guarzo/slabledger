import type { PostCardDetail, PostType } from '../../../../../types/social';
import { getTheme } from '../primitives/theme';
import { corsUrl } from '../primitives/corsUrl';

interface HotDealsCoverProps {
  postType: PostType;
  coverTitle: string;
  cards: PostCardDetail[];
  backgroundUrl?: string;
}

function formatPrice(cents: number): string {
  return `$${(cents / 100).toFixed(2)}`;
}

function discountPct(askingCents: number, marketCents: number): number {
  if (marketCents <= 0 || askingCents >= marketCents) return 0;
  return Math.round((1 - askingCents / marketCents) * 100);
}

export default function HotDealsCover({
  postType,
  coverTitle,
  cards,
  backgroundUrl,
}: HotDealsCoverProps) {
  const theme = getTheme(postType);
  const hero = cards[0];
  const left = cards[1];
  const right = cards[2];

  const discount = hero
    ? discountPct(hero.askingPriceCents, hero.clValueCents)
    : 0;

  const gradeLabel = hero
    ? `${hero.grader || 'PSA'} ${hero.gradeValue}`
    : '';

  return (
    <div className="absolute inset-0 flex flex-col" style={{ background: theme.coverBg }}>
      {/* Optional background image at 20% opacity */}
      {backgroundUrl && (
        <img
          src={corsUrl(backgroundUrl)}
          alt=""
          className="absolute inset-0 w-full h-full object-cover pointer-events-none"
          style={{ opacity: 0.2 }}
          crossOrigin="anonymous"
        />
      )}

      {/* Top gradient overlay for readability */}
      <div
        className="absolute inset-0 pointer-events-none"
        style={{
          background:
            'linear-gradient(180deg, rgba(0,0,0,0.55) 0%, transparent 35%, transparent 60%, rgba(0,0,0,0.75) 100%)',
        }}
      />

      {/* Glow behind hero card */}
      <div
        className="absolute left-1/2 top-[38%] -translate-x-1/2 -translate-y-1/2 w-[320px] h-[320px] rounded-full blur-3xl pointer-events-none"
        style={{ background: theme.coverGlow }}
      />

      {/* ── TOP BAR ── */}
      <div className="relative z-10 flex items-center justify-between px-5 pt-4 pb-3">
        {/* Logo + brand */}
        <div className="flex items-center gap-2">
          <img
            src="/card-yeti-business-logo.png"
            alt="Card Yeti"
            className="w-[36px] h-auto"
            crossOrigin="anonymous"
          />
          <span className="text-[11px] font-bold text-white/80 tracking-[1.5px]">
            CARD YETI
          </span>
        </div>

        {/* Hot Deals badge */}
        <span className="inline-flex items-center gap-1 bg-red-600 text-white text-[10px] font-bold tracking-wider rounded-full px-3 py-1.5 shadow-lg">
          🔥 HOT DEALS
        </span>
      </div>

      {/* ── CARD SHOWCASE ── */}
      <div className="relative flex-1 flex items-center justify-center z-[5]">
        {/* Left secondary card */}
        {left && (
          <div
            className="absolute rounded-xl overflow-hidden shadow-[0_8px_24px_rgba(0,0,0,0.6)]"
            style={{
              width: '32%',
              aspectRatio: '0.65',
              left: '-4%',
              top: '50%',
              transform: 'translateY(-50%) rotate(-8deg)',
              opacity: 0.55,
              zIndex: 2,
            }}
          >
            {left.frontImageUrl ? (
              <img
                src={corsUrl(left.frontImageUrl)}
                alt={left.cardName}
                className="w-full h-full object-cover"
                crossOrigin="anonymous"
              />
            ) : (
              <div className="w-full h-full bg-gray-700 flex items-center justify-center text-white/40 text-xs text-center px-2">
                {left.cardName}
              </div>
            )}
          </div>
        )}

        {/* Hero card (center) */}
        {hero && (
          <div
            className="relative rounded-2xl overflow-hidden shadow-[0_20px_50px_rgba(0,0,0,0.7)]"
            style={{ width: '42%', aspectRatio: '0.65', zIndex: 4 }}
          >
            {hero.frontImageUrl ? (
              <img
                src={corsUrl(hero.frontImageUrl)}
                alt={hero.cardName}
                className="w-full h-full object-cover"
                crossOrigin="anonymous"
              />
            ) : (
              <div className="w-full h-full bg-gray-700 flex items-center justify-center text-white/40 text-xs text-center px-2">
                {hero.cardName}
              </div>
            )}

            {/* Grade badge – top-right corner */}
            {gradeLabel && (
              <div className="absolute top-2 right-2 bg-red-600 text-white text-[9px] font-bold rounded-md px-2 py-1 leading-none shadow-md">
                {gradeLabel}
              </div>
            )}

            {/* Thin red bottom border accent */}
            <div className="absolute bottom-0 left-0 right-0 h-[3px] bg-red-500" />
          </div>
        )}

        {/* Right secondary card */}
        {right && (
          <div
            className="absolute rounded-xl overflow-hidden shadow-[0_8px_24px_rgba(0,0,0,0.6)]"
            style={{
              width: '32%',
              aspectRatio: '0.65',
              right: '-4%',
              top: '50%',
              transform: 'translateY(-50%) rotate(8deg)',
              opacity: 0.55,
              zIndex: 2,
            }}
          >
            {right.frontImageUrl ? (
              <img
                src={corsUrl(right.frontImageUrl)}
                alt={right.cardName}
                className="w-full h-full object-cover"
                crossOrigin="anonymous"
              />
            ) : (
              <div className="w-full h-full bg-gray-700 flex items-center justify-center text-white/40 text-xs text-center px-2">
                {right.cardName}
              </div>
            )}
          </div>
        )}
      </div>

      {/* ── BOTTOM INFO ── */}
      <div className="relative z-10 px-5 pb-5 pt-3 flex flex-col gap-1">
        {/* Price row */}
        {hero && (
          <div className="flex items-baseline gap-3 flex-wrap">
            {/* Asking price */}
            <span className="text-red-400 font-black text-[28px] leading-none">
              {formatPrice(hero.askingPriceCents)}
            </span>

            {/* Market price strikethrough */}
            {hero.clValueCents > 0 && (
              <span className="text-white/40 text-[14px] line-through">
                {formatPrice(hero.clValueCents)}
              </span>
            )}

            {/* Discount callout */}
            {discount > 0 && (
              <span className="inline-flex items-center bg-red-600/80 text-white text-[10px] font-bold rounded px-2 py-0.5 tracking-wide">
                {discount}% below market
              </span>
            )}
          </div>
        )}

        {/* Card name & grade */}
        {hero && (
          <div className="flex items-center gap-2 flex-wrap">
            <span className="text-white font-semibold text-[13px] leading-snug">
              {hero.cardName}
            </span>
            {gradeLabel && (
              <span className="text-white/50 text-[11px]">· {gradeLabel}</span>
            )}
          </div>
        )}

        {/* Cover title + card count + website */}
        <div className="flex items-center justify-between mt-0.5">
          <span className="text-white/60 text-[10px] font-medium tracking-wide">
            {coverTitle}
            {cards.length > 0 && (
              <span className="text-white/35 ml-1">· {cards.length} cards</span>
            )}
          </span>
          <span className="text-white/30 text-[9px]">cardyeti.com</span>
        </div>
      </div>
    </div>
  );
}
