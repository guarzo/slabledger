// Social slide grade badge using post-type gradient theming.
// For the general-purpose grade badge (inventory, campaign detail, etc.), see:
// web/src/react/ui/GradeBadge.tsx
// They are separate intentionally — this version uses post-type theming via getTheme().
import { getTheme } from './theme';
import type { PostType } from '../../../../../types/social';

interface GradeBadgeProps {
  grader: string;
  grade: number;
  postType: PostType;
}

export default function GradeBadge({ grader, grade, postType }: GradeBadgeProps) {
  const theme = getTheme(postType);
  return (
    <span className={`shrink-0 bg-gradient-to-r ${theme.gradientBar} rounded-md px-3 py-1 text-sm font-bold`}>
      {grader || 'PSA'} {grade}
    </span>
  );
}
