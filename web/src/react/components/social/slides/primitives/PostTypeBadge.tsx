import type { PostType } from '../../../../../types/social';
import { getTheme } from './theme';

const BADGE_ICONS: Record<PostType, string> = {
  hot_deals: '\uD83D\uDD25',
  new_arrivals: '\u2728',
  price_movers: '\uD83D\uDCC8',
};

export default function PostTypeBadge({ postType }: { postType: PostType }) {
  const theme = getTheme(postType);
  const icon = BADGE_ICONS[postType] ?? '';
  return (
    <span className={`inline-flex items-center border rounded-full px-3 py-1 text-[9px] font-semibold ${theme.accentBg}`}>
      {icon} {theme.label.toUpperCase()}
    </span>
  );
}
