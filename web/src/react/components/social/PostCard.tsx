import { CardShell } from '../../ui/CardShell';
import Button from '../../ui/Button';
import type { SocialPost, PostType, PostStatus } from '../../../types/social';

const POST_TYPE_CONFIG: Record<PostType, { label: string; color: string; bg: string }> = {
  new_arrivals: { label: 'New Arrivals', color: 'text-blue-400', bg: 'bg-blue-500/10' },
  price_movers: { label: 'Price Movers', color: 'text-amber-400', bg: 'bg-amber-500/10' },
  hot_deals: { label: 'Hot Deals', color: 'text-emerald-400', bg: 'bg-emerald-500/10' },
};

const STATUS_CONFIG: Record<PostStatus, { classes: string; label: string }> = {
  draft: { classes: 'bg-gray-500/10 text-gray-400', label: 'Draft' },
  publishing: { classes: 'bg-yellow-500/10 text-yellow-400 animate-pulse', label: 'Publishing...' },
  published: { classes: 'bg-purple-500/10 text-purple-400', label: 'Published' },
  failed: { classes: 'bg-red-500/10 text-red-400', label: 'Failed' },
  approved: { classes: 'bg-emerald-500/10 text-emerald-400', label: 'Approved' },
  rejected: { classes: 'bg-red-500/10 text-red-400', label: 'Rejected' },
};

interface PostCardProps {
  post: SocialPost;
  onPreview: () => void;
  onPublish?: () => void;
  onDelete: () => void;
}

export default function PostCard({ post, onPreview, onPublish, onDelete }: PostCardProps) {
  const typeConfig = POST_TYPE_CONFIG[post.postType] ?? POST_TYPE_CONFIG.new_arrivals;
  const statusConfig = STATUS_CONFIG[post.status] ?? STATUS_CONFIG.draft;
  const date = new Date(post.createdAt).toLocaleDateString('en-US', {
    month: 'short',
    day: 'numeric',
    year: 'numeric',
  });

  const captionPreview = post.caption
    ? post.caption.length > 150 ? post.caption.slice(0, 150) + '...' : post.caption
    : 'No caption generated';

  const isPublishable = post.status === 'draft' || post.status === 'failed' || post.status === 'approved';
  const isDeletable = post.status !== 'publishing';

  return (
    <CardShell variant="interactive" onClick={onPreview}>
      <div className="flex items-start justify-between gap-4">
        <div className="flex-1 min-w-0">
          <div className="flex items-center gap-2 mb-2">
            <span className={`text-xs font-medium px-2 py-0.5 rounded-full ${typeConfig.bg} ${typeConfig.color}`}>
              {typeConfig.label}
            </span>
            <span className={`text-xs font-medium px-2 py-0.5 rounded-full ${statusConfig.classes}`}>
              {statusConfig.label}
            </span>
            <span className="text-xs text-[var(--text-muted)]">{post.cardCount} cards</span>
            <span className="text-xs text-[var(--text-muted)]">{date}</span>
          </div>
          <h3 className="text-sm font-medium text-[var(--text)] mb-1">{post.coverTitle}</h3>
          <p className="text-xs text-[var(--text-muted)] line-clamp-2">{captionPreview}</p>
          {post.status === 'failed' && post.errorMessage && (
            <p className="text-xs text-red-400 mt-1">{post.errorMessage}</p>
          )}
        </div>

        <div className="flex items-center gap-2 shrink-0" onClick={(e) => e.stopPropagation()}>
          {isPublishable && onPublish && (
            <Button size="sm" variant="primary" onClick={onPublish}>
              {post.status === 'failed' ? 'Retry' : 'Publish'}
            </Button>
          )}
          {isDeletable && (
            <Button size="sm" variant="danger" onClick={onDelete}>Delete</Button>
          )}
        </div>
      </div>
    </CardShell>
  );
}
