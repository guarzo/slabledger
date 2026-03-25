import { useState, useCallback } from 'react';
import { useSocialPost } from '../../queries/useSocialQueries';
import { usePublishWithSlides } from '../../hooks/usePublishWithSlides';
import { useToast } from '../../contexts/ToastContext';
import { CardShell } from '../../ui/CardShell';
import Button from '../../ui/Button';
import CaptionEditor from './CaptionEditor';
import CoverSlide from './slides/CoverSlide';
import CardSlide from './slides/CardSlide';
import SlideRenderer from './slides/SlideRenderer';
import type { PostCardDetail } from '../../../types/social';

interface PostPreviewProps {
  postId: string;
  onBack: () => void;
  igConnected?: boolean;
}

export default function PostPreview({ postId, onBack, igConnected }: PostPreviewProps) {
  const { data: detail, isLoading } = useSocialPost(postId);
  const [currentSlide, setCurrentSlide] = useState(0);
  const [coverTitleOverride, setCoverTitleOverride] = useState<string | null>(null);
  const { publish, isPublishing, progress } = usePublishWithSlides(detail);
  const toast = useToast();
  const handleTitleChange = useCallback((title: string) => {
    setCoverTitleOverride(title);
  }, []);

  if (isLoading || !detail) {
    return (
      <CardShell padding="lg">
        <p className="text-[var(--text-muted)] text-center">Loading post...</p>
      </CardShell>
    );
  }

  const totalSlides = (detail.cards?.length ?? 0) + 1;
  const psa10Count = detail.cards?.filter(c => c.gradeValue === 10).length ?? 0;
  const totalValueCents = detail.cards?.reduce((sum, c) => sum + c.medianCents, 0) ?? 0;
  const isPublishable = detail.status === 'draft' || detail.status === 'failed' || detail.status === 'approved';

  const handlePublish = async () => {
    try {
      await publish();
    } catch (err) {
      toast.error(err instanceof Error ? err.message : 'Failed to publish');
    }
  };

  return (
    <div className="space-y-6">
      {/* Header */}
      <div className="flex items-center justify-between">
        <button
          onClick={onBack}
          className="text-sm text-[var(--text-muted)] hover:text-[var(--text)] transition-colors"
        >
          &larr; Back to posts
        </button>
        <div className="flex gap-2">
          <SlideRenderer post={detail} />
          {isPublishable && igConnected && (
            <Button
              size="sm"
              variant="primary"
              onClick={handlePublish}
              disabled={isPublishing}
              loading={isPublishing}
            >
              {isPublishing && progress
                ? progress
                : detail.status === 'failed'
                  ? 'Retry Publish'
                  : 'Publish'}
            </Button>
          )}
          {detail.status === 'published' && (
            <span className="text-xs bg-purple-500/10 text-purple-400 px-3 py-2 rounded-lg font-medium">
              Published
            </span>
          )}
        </div>
      </div>

      {detail.status === 'failed' && detail.errorMessage && (
        <div className="bg-red-500/10 border border-red-500/20 rounded-lg px-4 py-3 text-sm text-red-400">
          {detail.errorMessage}
        </div>
      )}

      <div className="grid grid-cols-1 lg:grid-cols-2 gap-6">
        {/* Slide preview */}
        <CardShell padding="none">
          <div className="p-4">
            <div className="aspect-square w-full max-w-[540px] mx-auto bg-[var(--surface-0)] rounded-lg overflow-hidden">
              {currentSlide === 0 ? (
                <CoverSlide
                  postType={detail.postType}
                  coverTitle={coverTitleOverride ?? detail.coverTitle}
                  cardCount={detail.cardCount}
                  createdAt={detail.createdAt}
                  psa10Count={psa10Count}
                  totalValueCents={totalValueCents}
                />
              ) : (
                detail.cards && detail.cards[currentSlide - 1] && (
                  <CardSlide
                    card={detail.cards[currentSlide - 1] as PostCardDetail}
                    postType={detail.postType}
                    slideIndex={currentSlide}
                    totalSlides={totalSlides}
                  />
                )
              )}
            </div>

            {/* Slide navigation */}
            <div className="flex items-center justify-center gap-2 mt-4">
              <button
                onClick={() => setCurrentSlide(Math.max(0, currentSlide - 1))}
                disabled={currentSlide === 0}
                className="text-[var(--text-muted)] hover:text-[var(--text)] disabled:opacity-30 transition-colors"
              >
                &larr;
              </button>
              <div className="flex gap-1">
                {Array.from({ length: totalSlides }, (_, i) => (
                  <button
                    key={i}
                    onClick={() => setCurrentSlide(i)}
                    className={`w-2 h-2 rounded-full transition-colors ${
                      i === currentSlide ? 'bg-[var(--brand-500)]' : 'bg-[var(--surface-2)]'
                    }`}
                  />
                ))}
              </div>
              <button
                onClick={() => setCurrentSlide(Math.min(totalSlides - 1, currentSlide + 1))}
                disabled={currentSlide === totalSlides - 1}
                className="text-[var(--text-muted)] hover:text-[var(--text)] disabled:opacity-30 transition-colors"
              >
                &rarr;
              </button>
              <span className="text-xs text-[var(--text-muted)] ml-2">
                {currentSlide + 1} / {totalSlides}
              </span>
            </div>
          </div>
        </CardShell>

        {/* Caption editor */}
        <CaptionEditor
          postId={postId}
          caption={detail.caption}
          hashtags={detail.hashtags}
          onTitleChange={handleTitleChange}
        />
      </div>
    </div>
  );
}
