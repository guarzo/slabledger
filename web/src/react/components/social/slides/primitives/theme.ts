import type { PostType } from '../../../../../types/social';

export interface SlideTheme {
  label: string;
  accentPrimary: string;
  accentBg: string;
  gradientBar: string;
  glowColor: string;
  overlayAccent: string;
}

export const SLIDE_THEMES: Record<PostType, SlideTheme> = {
  hot_deals: {
    label: 'Hot Deals',
    accentPrimary: 'text-emerald-400',
    accentBg: 'bg-red-500/20 border-red-500/40 text-red-300',
    gradientBar: 'from-transparent via-emerald-500 to-teal-400',
    glowColor: 'rgba(239,68,68,0.15)',
    overlayAccent: 'rgba(239,68,68,0.08)',
  },
  new_arrivals: {
    label: 'New Arrivals',
    accentPrimary: 'text-indigo-400',
    accentBg: 'bg-indigo-500/15 border-indigo-500/30 text-indigo-300',
    gradientBar: 'from-transparent via-indigo-500 to-emerald-500',
    glowColor: 'rgba(99,102,241,0.15)',
    overlayAccent: 'rgba(99,102,241,0.08)',
  },
  price_movers: {
    label: 'Trending',
    accentPrimary: 'text-amber-400',
    accentBg: 'bg-amber-500/15 border-amber-500/30 text-amber-300',
    gradientBar: 'from-transparent via-amber-500 to-red-500',
    glowColor: 'rgba(245,158,11,0.15)',
    overlayAccent: 'rgba(245,158,11,0.08)',
  },
};

export function getTheme(postType: PostType): SlideTheme {
  return SLIDE_THEMES[postType] ?? SLIDE_THEMES.new_arrivals;
}
