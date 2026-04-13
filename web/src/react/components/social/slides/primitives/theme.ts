import type { PostType } from '../../../../../types/social';

export interface SlideTheme {
  label: string;
  accentPrimary: string;
  accentBg: string;
  gradientBar: string;
  glowColor: string;
  overlayAccent: string;
  // Cover-specific
  coverBg: string;          // CSS gradient for cover background
  coverAccent: string;      // Primary accent hex color for cover elements
  coverGlow: string;        // Radial glow rgba for behind hero card
  coverBadgeLabel: string;  // Label shown on cover (e.g. "JUST LANDED")
}

export const SLIDE_THEMES: Record<PostType, SlideTheme> = {
  hot_deals: {
    label: 'Hot Deals',
    accentPrimary: 'text-emerald-400',
    accentBg: 'bg-red-500/20 border-red-500/40 text-red-300',
    gradientBar: 'from-transparent via-emerald-500 to-teal-400',
    glowColor: 'rgba(239,68,68,0.15)',
    overlayAccent: 'rgba(239,68,68,0.08)',
    coverBg: 'linear-gradient(135deg, #1a0a0a 0%, #0a0e1a 40%, #0a1a0a 100%)',
    coverAccent: '#dc2626',
    coverGlow: 'rgba(220, 38, 38, 0.15)',
    coverBadgeLabel: '',
  },
  new_arrivals: {
    label: 'New Arrivals',
    accentPrimary: 'text-indigo-400',
    accentBg: 'bg-indigo-500/15 border-indigo-500/30 text-indigo-300',
    gradientBar: 'from-transparent via-indigo-500 to-emerald-500',
    glowColor: 'rgba(99,102,241,0.15)',
    overlayAccent: 'rgba(99,102,241,0.08)',
    coverBg: 'linear-gradient(160deg, #0a0e1a 0%, #0a1a2a 50%, #0a1a1a 100%)',
    coverAccent: '#0d9488',
    coverGlow: 'rgba(13, 148, 136, 0.12)',
    coverBadgeLabel: 'JUST LANDED',
  },
  price_movers: {
    label: 'Trending',
    accentPrimary: 'text-amber-400',
    accentBg: 'bg-amber-500/15 border-amber-500/30 text-amber-300',
    gradientBar: 'from-transparent via-amber-500 to-red-500',
    glowColor: 'rgba(245,158,11,0.15)',
    overlayAccent: 'rgba(245,158,11,0.08)',
    coverBg: 'linear-gradient(145deg, #1a1508 0%, #0a0e1a 50%, #1a0a08 100%)',
    coverAccent: '#d97706',
    coverGlow: 'rgba(217, 119, 6, 0.1)',
    coverBadgeLabel: 'TRENDING UP',
  },
  dh_instagram: {
    label: 'DoubleHolo',
    accentPrimary: 'text-purple-400',
    accentBg: 'bg-purple-500/15 border-purple-500/30 text-purple-300',
    gradientBar: 'from-transparent via-purple-500 to-indigo-400',
    glowColor: 'rgba(168,85,247,0.15)',
    overlayAccent: 'rgba(168,85,247,0.08)',
    coverBg: 'linear-gradient(150deg, #0e0a1a 0%, #0a0e1a 50%, #0a0a1a 100%)',
    coverAccent: '#9333ea',
    coverGlow: 'rgba(147, 51, 234, 0.12)',
    coverBadgeLabel: 'DOUBLEHOLO',
  },
};

export function getTheme(postType: PostType): SlideTheme {
  return SLIDE_THEMES[postType] ?? SLIDE_THEMES.new_arrivals;
}
