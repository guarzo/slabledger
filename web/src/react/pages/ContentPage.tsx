import { useMemo, useState } from 'react';
import { useSocialPosts, useGenerateSocialPosts, useDeleteSocialPost, useInstagramStatus } from '../queries/useSocialQueries';
import { useToast } from '../contexts/ToastContext';
import type { SocialPost } from '../../types/social';
import PostCard from '../components/social/PostCard';
import PostPreview from '../components/social/PostPreview';
import { CardShell } from '../ui/CardShell';
import Button from '../ui/Button';

export default function ContentPage() {
  const [previewPostId, setPreviewPostId] = useState<string | null>(null);
  const [showPublished, setShowPublished] = useState(false);
  const toast = useToast();

  const { data: posts, isLoading } = useSocialPosts();
  const generateMutation = useGenerateSocialPosts();
  const deleteMutation = useDeleteSocialPost();
  const { data: igStatus } = useInstagramStatus();

  const handleGenerate = async () => {
    try {
      await generateMutation.mutateAsync();
      toast.success('Generating posts... new content will appear shortly');
    } catch (err) {
      toast.error(err instanceof Error ? err.message : 'Failed to generate posts');
    }
  };

  const handleDelete = async (id: string) => {
    try {
      await deleteMutation.mutateAsync(id);
      toast.success('Post deleted');
    } catch (err) {
      toast.error(err instanceof Error ? err.message : 'Failed to delete post');
    }
  };

  const filteredPosts = useMemo(
    () => (posts ?? []).filter((post: SocialPost) => showPublished || post.status !== 'published'),
    [posts, showPublished],
  );

  if (previewPostId) {
    return (
      <div className="max-w-6xl mx-auto px-4">
        <PostPreview
          postId={previewPostId}
          onBack={() => setPreviewPostId(null)}
          igConnected={igStatus?.connected}
        />
      </div>
    );
  }

  return (
    <div className="max-w-6xl mx-auto px-4">
      <div className="flex items-center justify-between mb-6">
        <div className="flex items-center gap-3">
          <h1 className="text-2xl font-bold text-gradient text-gradient-premium">Content</h1>
          {igStatus && (
            <span className={`text-xs px-2 py-0.5 rounded-full ${
              igStatus.connected
                ? 'bg-emerald-500/10 text-emerald-400'
                : 'bg-gray-500/10 text-gray-400'
            }`}>
              {igStatus.connected ? `@${igStatus.username}` : 'IG not connected'}
            </span>
          )}
        </div>
        <Button
          onClick={handleGenerate}
          loading={generateMutation.isPending}
          variant="primary"
          size="sm"
        >
          Generate Posts
        </Button>
      </div>

      {posts && posts.length > 0 && (
        <div className="flex items-center gap-3 mb-4">
          {[
            { label: 'Drafts', active: !showPublished, onClick: () => setShowPublished(false) },
            { label: 'All Posts', active: showPublished, onClick: () => setShowPublished(true) },
          ].map(btn => (
            <button
              key={btn.label}
              type="button"
              onClick={btn.onClick}
              className={`text-xs font-medium px-3 py-1.5 rounded-full border transition-colors ${
                btn.active
                  ? 'border-[var(--brand-500)] bg-[var(--brand-500)]/10 text-[var(--brand-400)]'
                  : 'border-[var(--surface-2)] text-[var(--text-muted)] hover:text-[var(--text)]'
              }`}
            >
              {btn.label}
            </button>
          ))}
        </div>
      )}

      {/* Post list — flat list with draft/published filter */}
      {isLoading ? (
        <CardShell padding="lg">
          <p className="text-[var(--text-muted)] text-center">Loading posts...</p>
        </CardShell>
      ) : !posts || posts.length === 0 ? (
        <CardShell padding="lg">
          <div className="text-center py-8">
            <p className="text-[var(--text-muted)] mb-4">
              No posts yet. Click &quot;Generate Posts&quot; to create content from your inventory.
            </p>
          </div>
        </CardShell>
      ) : filteredPosts.length === 0 ? (
        <CardShell padding="lg">
          <div className="text-center py-8">
            <p className="text-[var(--text-muted)] mb-4">
              {showPublished ? 'No posts yet.' : 'No draft posts. All posts have been published.'}
            </p>
            {!showPublished && (
              <button
                type="button"
                onClick={() => setShowPublished(true)}
                className="text-sm text-[var(--brand-400)] hover:text-[var(--brand-300)]"
              >
                Show all posts
              </button>
            )}
          </div>
        </CardShell>
      ) : (
        <div className="grid gap-4">
          {filteredPosts.map((post: SocialPost) => (
            <PostCard
              key={post.id}
              post={post}
              onPreview={() => setPreviewPostId(post.id)}
              onDelete={() => handleDelete(post.id)}
            />
          ))}
        </div>
      )}
    </div>
  );
}
