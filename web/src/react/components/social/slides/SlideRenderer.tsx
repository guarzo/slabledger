import { useCallback } from 'react';
import { useToast } from '../../../contexts/ToastContext';
import Button from '../../../ui/Button';
import type { SocialPostDetail } from '../../../../types/social';

interface SlideRendererProps {
  post: SocialPostDetail;
}

export default function SlideRenderer({ post }: SlideRendererProps) {
  const toast = useToast();

  const handleDownloadSlides = useCallback(async () => {
    try {
      // Dynamically import html-to-image only when needed
      const { toPng } = await import('html-to-image');

      // Find all slide elements currently rendered in the DOM
      const slideElements = document.querySelectorAll('[data-slide]');
      if (slideElements.length === 0) {
        toast.error('No slides to download. Navigate through slides first.');
        return;
      }

      // Download the currently visible slide
      const currentSlide = slideElements[0] as HTMLElement;
      const dataUrl = await toPng(currentSlide, {
        width: 1080,
        height: 1080,
        pixelRatio: 2,
        backgroundColor: '#0d1117',
      });

      const link = document.createElement('a');
      link.download = `card-yeti-${post.postType}-${post.id}-slide.png`;
      link.href = dataUrl;
      link.click();

      toast.success('Slide downloaded');
    } catch {
      toast.error('Failed to download slide. Try using a screenshot instead.');
    }
  }, [post, toast]);

  const handleCopyCaption = useCallback(async () => {
    const fullText = post.hashtags
      ? `${post.caption}\n\n${post.hashtags}`
      : post.caption;
    try {
      await navigator.clipboard.writeText(fullText);
      toast.success('Caption copied to clipboard');
    } catch {
      toast.error('Failed to copy caption');
    }
  }, [post, toast]);

  return (
    <div className="flex gap-2">
      <Button size="sm" variant="ghost" onClick={handleDownloadSlides}>
        Download Slide
      </Button>
      <Button size="sm" variant="ghost" onClick={handleCopyCaption}>
        Copy Caption
      </Button>
    </div>
  );
}
