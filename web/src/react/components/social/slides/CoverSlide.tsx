import type { PostType } from '../../../../types/social';
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

interface CoverSlideProps {
  postType: PostType;
  coverTitle: string;
  cardCount: number;
  createdAt: string;
  psa10Count: number;
  totalValueCents: number;
}

function formatValue(cents: number): string {
  const dollars = cents / 100;
  if (dollars >= 1000) {
    return `$${(dollars / 1000).toFixed(1)}k`;
  }
  return `$${dollars.toFixed(0)}`;
}

export default function CoverSlide({
  postType,
  coverTitle,
  cardCount,
  createdAt,
  psa10Count,
  totalValueCents,
}: CoverSlideProps) {
  const theme = THEME[postType] ?? THEME.new_arrivals;
  const date = new Date(createdAt).toLocaleDateString('en-US', {
    month: 'long',
    day: 'numeric',
    year: 'numeric',
  });

  return (
    <div
      className="w-full h-full bg-gradient-to-br from-[#0a0e1a] via-[#111827] to-[#1a1f35] flex flex-col items-center justify-center p-8 text-white relative overflow-hidden"
      data-slide="cover"
    >
      {/* Accent bar */}
      <div className={`absolute top-0 left-0 right-0 h-[3px] bg-gradient-to-r ${theme.gradientBar}`} />

      {/* Radial glow overlays */}
      <div className="absolute top-1/4 left-1/2 -translate-x-1/2 w-[400px] h-[400px] rounded-full bg-indigo-500/[0.06] blur-3xl pointer-events-none" />
      <div className="absolute bottom-1/4 right-1/4 w-[300px] h-[300px] rounded-full bg-emerald-500/[0.04] blur-3xl pointer-events-none" />

      {/* Logo */}
      <img src={logo} alt="Card Yeti" className="w-[200px] mb-2" />

      {/* Subtitle */}
      <p className="text-[10px] uppercase tracking-[0.2em] text-white/50 mb-6">
        PSA Graded Pokemon
      </p>

      {/* Post-type badge */}
      <span className={`inline-flex items-center border rounded-full px-4 py-1 text-xs font-medium mb-4 ${theme.accentBg}`}>
        {theme.label}
      </span>

      {/* Title */}
      <h2 className="text-2xl font-bold text-center mb-2 max-w-[90%]">{coverTitle}</h2>

      {/* Date */}
      <p className="text-sm text-white/50 mb-6">{date}</p>

      {/* Stats row */}
      <div className="flex gap-3 mb-6">
        <div className="bg-white/5 border border-white/10 rounded-lg px-4 py-3 text-center min-w-[90px]">
          <div className="text-xl font-bold">{cardCount}</div>
          <div className="text-[10px] uppercase tracking-wider text-white/50">Cards</div>
        </div>
        <div className="bg-white/5 border border-white/10 rounded-lg px-4 py-3 text-center min-w-[90px]">
          <div className={`text-xl font-bold ${theme.accent}`}>{psa10Count}</div>
          <div className="text-[10px] uppercase tracking-wider text-white/50">PSA 10</div>
        </div>
        <div className="bg-white/5 border border-white/10 rounded-lg px-4 py-3 text-center min-w-[90px]">
          <div className={`text-xl font-bold ${theme.accent}`}>{formatValue(totalValueCents)}</div>
          <div className="text-[10px] uppercase tracking-wider text-white/50">Value</div>
        </div>
      </div>

      {/* Footer */}
      <div className="mt-auto text-xs text-white/40">cardyeti.com</div>
    </div>
  );
}
