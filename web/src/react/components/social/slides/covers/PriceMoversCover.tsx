import type { PostCardDetail, PostType } from '../../../../../types/social';
import { getTheme } from '../primitives/theme';
import { corsUrl } from '../primitives/corsUrl';

interface PriceMoversCoverProps {
  postType: PostType;
  coverTitle: string;
  cards: PostCardDetail[];
  backgroundUrl?: string;
}

export default function PriceMoversCover({
  postType,
  coverTitle,
  cards,
  backgroundUrl,
}: PriceMoversCoverProps) {
  const theme = getTheme(postType);
  if (cards.length === 0) return null;
  const hero = cards[0];

  const gradeLabel = hero
    ? `${hero.grader || 'PSA'} ${hero.gradeValue}`
    : '';

  const trendUp = hero ? hero.trend30d > 0 : true;
  const trendPct = hero ? Math.round(hero.trend30d * 100) : 0;
  const trendDisplay = trendPct >= 0 ? `+${trendPct}%` : `${trendPct}%`;
  const trendColor = trendUp ? '#22c55e' : '#ef4444';

  const svgPoints = trendUp
    ? '0,28 15,25 30,22 45,18 55,12 65,8 80,4'
    : '0,4 15,8 30,12 45,18 55,22 65,25 80,28';

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

      {/* Gradient overlay for readability */}
      <div
        className="absolute inset-0 pointer-events-none"
        style={{
          background:
            'linear-gradient(180deg, rgba(0,0,0,0.55) 0%, transparent 30%, transparent 55%, rgba(0,0,0,0.80) 100%)',
        }}
      />

      {/* Glow behind hero card */}
      <div
        className="absolute top-[40%] -translate-y-1/2 w-[320px] h-[320px] rounded-full blur-3xl pointer-events-none"
        style={{ left: '22%', transform: 'translate(-50%, -50%)', background: theme.coverGlow }}
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

        {/* Price Movers badge */}
        <span
          className="inline-flex items-center gap-1 text-white text-[10px] font-bold tracking-wider rounded-full px-3 py-1.5 shadow-lg"
          style={{ backgroundColor: theme.coverAccent }}
        >
          📈 PRICE MOVERS
        </span>
      </div>

      {/* ── MAIN CONTENT: Hero card LEFT + Trend data RIGHT ── */}
      <div className="relative flex-1 flex items-center z-[5] px-5 gap-6">
        {/* Hero card — ~45% width */}
        {hero && (
          <div
            className="relative rounded-2xl overflow-hidden flex-shrink-0"
            style={{
              width: '45%',
              aspectRatio: '0.65',
              boxShadow: `0 0 0 2px ${theme.coverAccent}, 0 0 32px 8px ${theme.coverGlow}, 0 20px 50px rgba(0,0,0,0.7)`,
            }}
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

            {/* Grade badge — top-right, amber background */}
            {gradeLabel && (
              <div
                className="absolute top-2 right-2 text-white text-[9px] font-bold rounded-md px-2 py-1 leading-none shadow-md"
                style={{ backgroundColor: '#d97706' }}
              >
                {gradeLabel}
              </div>
            )}

            {/* Amber bottom border accent */}
            <div
              className="absolute bottom-0 left-0 right-0 h-[3px]"
              style={{ backgroundColor: theme.coverAccent }}
            />
          </div>
        )}

        {/* Trend data — right side */}
        <div className="flex flex-col items-start justify-center gap-3 flex-1">
          {/* Large trend percentage */}
          <span
            className="font-black leading-none"
            style={{ fontSize: '56px', color: trendColor }}
          >
            {trendDisplay}
          </span>

          {/* "30-day trend" label */}
          <span className="text-white/50 text-[13px] font-medium tracking-wide">
            30-day trend
          </span>

          {/* Mini SVG trend line chart */}
          <svg
            width="80"
            height="30"
            viewBox="0 0 80 30"
            fill="none"
            xmlns="http://www.w3.org/2000/svg"
          >
            <polyline
              points={svgPoints}
              stroke={trendColor}
              strokeWidth="2.5"
              strokeLinecap="round"
              strokeLinejoin="round"
              fill="none"
            />
          </svg>
        </div>
      </div>

      {/* ── BOTTOM INFO ── */}
      <div className="relative z-10 px-5 pb-5 pt-3 flex flex-col gap-1">
        {/* TRENDING UP / TRENDING DOWN label */}
        <span
          className="text-[10px] font-black tracking-[2px] uppercase"
          style={{ color: '#fbbf24' }}
        >
          {trendUp ? 'TRENDING UP' : 'TRENDING DOWN'}
        </span>

        {/* Hero card name */}
        {hero && (
          <span className="text-white font-semibold text-[15px] leading-snug">
            {hero.cardName}
          </span>
        )}

        {/* Card count teaser */}
        {cards.length > 1 && (
          <span className="text-white/55 text-[11px]">
            {cards.length} cards on the move
          </span>
        )}

        {/* Cover title + website */}
        <div className="flex items-center justify-between mt-0.5">
          <span className="text-white/50 text-[10px] font-medium tracking-wide">
            {coverTitle}
          </span>
          <span className="text-white/30 text-[9px]">cardyeti.com</span>
        </div>
      </div>
    </div>
  );
}
