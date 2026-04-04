import type { PostMetricsSnapshot } from '../../../types/social';

interface MetricsBarProps {
  latest: PostMetricsSnapshot | undefined;
}

export default function MetricsBar({ latest }: MetricsBarProps) {
  if (!latest) return null;

  return (
    <div className="flex items-center gap-3 text-xs text-white/50 mt-1">
      <span title="Likes" aria-label={`${latest.likes} likes`}>♥ {latest.likes}</span>
      <span title="Comments" aria-label={`${latest.comments} comments`}>💬 {latest.comments}</span>
      <span title="Saves" aria-label={`${latest.saves} saves`}>🔖 {latest.saves}</span>
      <span title="Reach" aria-label={`${latest.reach} reach`}>👁 {latest.reach}</span>
    </div>
  );
}
