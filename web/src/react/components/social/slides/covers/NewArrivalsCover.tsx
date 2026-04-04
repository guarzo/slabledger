import type { PostCardDetail, PostType } from '../../../../../types/social';
import { getTheme } from '../primitives/theme';
import { corsUrl } from '../primitives/corsUrl';

interface NewArrivalsCoverProps {
  postType: PostType;
  coverTitle: string;
  cards: PostCardDetail[];
  backgroundUrl?: string;
}

// Sparkle dot positions: [left%, top%, size(px), opacity]
const SPARKLE_DOTS: [number, number, number, number][] = [
  [12, 18, 5, 0.7],
  [22, 55, 3, 0.5],
  [8, 72, 4, 0.6],
  [18, 85, 3, 0.4],
  [35, 10, 4, 0.65],
  [48, 6, 3, 0.5],
  [62, 12, 5, 0.7],
  [78, 22, 3, 0.45],
  [88, 45, 4, 0.6],
  [82, 68, 3, 0.5],
  [90, 82, 5, 0.65],
  [68, 88, 3, 0.4],
  [38, 92, 4, 0.55],
  [55, 80, 3, 0.45],
  [72, 6, 4, 0.6],
];

export default function NewArrivalsCover({
  postType,
  coverTitle,
  cards,
  backgroundUrl,
}: NewArrivalsCoverProps) {
  const theme = getTheme(postType);
  const hero = cards[0];
  const extraCount = cards.length - 1;

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

      {/* Gradient overlay for readability */}
      <div
        className="absolute inset-0 pointer-events-none"
        style={{
          background:
            'linear-gradient(180deg, rgba(0,0,0,0.50) 0%, transparent 30%, transparent 55%, rgba(0,0,0,0.80) 100%)',
        }}
      />

      {/* Sparkle dots scattered around the hero */}
      {SPARKLE_DOTS.map(([left, top, size, opacity], i) => (
        <div
          key={i}
          className="absolute rounded-full pointer-events-none"
          style={{
            left: `${left}%`,
            top: `${top}%`,
            width: size,
            height: size,
            backgroundColor: theme.coverAccent,
            opacity,
          }}
        />
      ))}

      {/* Spotlight glow behind hero card */}
      <div
        className="absolute left-1/2 top-[40%] -translate-x-1/2 -translate-y-1/2 w-[380px] h-[380px] rounded-full blur-3xl pointer-events-none"
        style={{ background: `rgba(13, 148, 136, 0.18)` }}
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

        {/* New Arrivals badge */}
        <span
          className="inline-flex items-center gap-1 text-white text-[10px] font-bold tracking-wider rounded-full px-3 py-1.5 shadow-lg"
          style={{ backgroundColor: theme.coverAccent }}
        >
          ✨ NEW ARRIVALS
        </span>
      </div>

      {/* ── HERO CARD SHOWCASE ── */}
      <div className="relative flex-1 flex items-center justify-center z-[5]">
        {hero && (
          <div
            className="relative rounded-2xl overflow-hidden"
            style={{
              width: '46%',
              aspectRatio: '0.65',
              zIndex: 4,
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

            {/* Grade badge – top-right corner */}
            {gradeLabel && (
              <div
                className="absolute top-2 right-2 text-white text-[9px] font-bold rounded-md px-2 py-1 leading-none shadow-md"
                style={{ backgroundColor: theme.coverAccent }}
              >
                {gradeLabel}
              </div>
            )}

            {/* Teal bottom border accent */}
            <div
              className="absolute bottom-0 left-0 right-0 h-[3px]"
              style={{ backgroundColor: theme.coverAccent }}
            />
          </div>
        )}
      </div>

      {/* ── BOTTOM INFO ── */}
      <div className="relative z-10 px-5 pb-5 pt-3 flex flex-col gap-1">
        {/* JUST LANDED label */}
        <span
          className="text-[10px] font-black tracking-[2px] uppercase"
          style={{ color: theme.coverAccent }}
        >
          {theme.coverBadgeLabel || 'JUST LANDED'}
        </span>

        {/* Hero card name */}
        {hero && (
          <span className="text-white font-semibold text-[15px] leading-snug">
            {hero.cardName}
          </span>
        )}

        {/* "+ N more new slabs" teaser */}
        {extraCount > 0 && (
          <span className="text-white/55 text-[11px]">
            + {extraCount} more new slab{extraCount !== 1 ? 's' : ''}
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
