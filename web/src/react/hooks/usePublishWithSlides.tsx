import { useState, useCallback, useRef } from 'react';
import { createRoot } from 'react-dom/client';
import { useQueryClient } from '@tanstack/react-query';
import { api } from '../../js/api';
import { queryKeys } from '../queries/queryKeys';
import CoverSlide from '../components/social/slides/CoverSlide';
import CardSlide from '../components/social/slides/CardSlide';
import type { SocialPostDetail, PostCardDetail } from '../../types/social';

interface UsePublishWithSlidesResult {
  publish: () => Promise<void>;
  isPublishing: boolean;
  progress: string;
}

async function renderSlideToJpeg(
  element: React.ReactElement,
): Promise<Blob> {
  const { toJpeg } = await import('html-to-image');

  // Create offscreen container at Instagram's native 1080x1080
  const container = document.createElement('div');
  container.style.cssText =
    'position:fixed;left:-9999px;top:0;width:1080px;height:1080px;overflow:hidden;';
  document.body.appendChild(container);

  // Mount the React component
  const root = createRoot(container);
  root.render(element);

  // Wait for all images in the container to finish loading (or fail), then
  // allow one extra frame for the browser to paint.
  await Promise.all(
    Array.from(container.querySelectorAll('img')).map(
      (img) =>
        img.complete
          ? Promise.resolve()
          : new Promise<void>((resolve) => {
              img.addEventListener('load', () => resolve(), { once: true });
              img.addEventListener('error', () => resolve(), { once: true });
            }),
    ),
  );
  // Fallback minimum wait to handle images added after initial render
  await new Promise((resolve) => setTimeout(resolve, 200));

  try {
    const dataUrl = await toJpeg(container, {
      width: 1080,
      height: 1080,
      pixelRatio: 2,
      quality: 0.92,
      backgroundColor: '#0a0e1a',
    });

    // Convert data URL to Blob without fetch (avoids CSP connect-src restriction)
    const [header, base64] = dataUrl.split(',');
    const mime = header.match(/:(.*?);/)?.[1] ?? 'image/jpeg';
    const bytes = atob(base64);
    const buf = new Uint8Array(bytes.length);
    for (let i = 0; i < bytes.length; i++) buf[i] = bytes.charCodeAt(i);
    return new Blob([buf], { type: mime });
  } finally {
    root.unmount();
    document.body.removeChild(container);
  }
}

export function usePublishWithSlides(
  detail: SocialPostDetail | undefined,
): UsePublishWithSlidesResult {
  const [isPublishing, setIsPublishing] = useState(false);
  const [progress, setProgress] = useState('');
  const isPublishingRef = useRef(false);
  const queryClient = useQueryClient();

  const publish = useCallback(async () => {
    if (!detail || !detail.cards?.length) return;
    if (isPublishingRef.current) return;

    isPublishingRef.current = true;
    setIsPublishing(true);
    try {
      const totalSlides = detail.cards.length + 1;
      const psa10Count = detail.cards.filter((c) => c.gradeValue === 10).length;
      const totalValueCents = detail.cards.reduce(
        (sum, c) => sum + c.medianCents,
        0,
      );
      const slides: Blob[] = [];

      // Render cover slide
      setProgress('Rendering cover slide...');
      const coverBlob = await renderSlideToJpeg(
        <CoverSlide
          postType={detail.postType}
          coverTitle={detail.coverTitle}
          cardCount={detail.cardCount}
          createdAt={detail.createdAt}
          psa10Count={psa10Count}
          totalValueCents={totalValueCents}
          cards={detail.cards}
        />,
      );
      slides.push(coverBlob);

      // Render each card slide
      for (let i = 0; i < detail.cards.length; i++) {
        setProgress(`Rendering slide ${i + 2} of ${totalSlides}...`);
        const cardBlob = await renderSlideToJpeg(
          <CardSlide
            card={detail.cards[i] as PostCardDetail}
            postType={detail.postType}
            slideIndex={i + 1}
            totalSlides={totalSlides}
          />,
        );
        slides.push(cardBlob);
      }

      // Upload slides
      setProgress('Uploading slides...');
      await api.uploadSlides(detail.id, slides);

      // Publish to Instagram
      setProgress('Publishing to Instagram...');
      await api.publishSocialPost(detail.id);

      // Invalidate queries to pick up new status
      queryClient.invalidateQueries({ queryKey: queryKeys.social.all });

      setProgress('Published!');
    } catch (error) {
      setProgress('');
      throw error;
    } finally {
      isPublishingRef.current = false;
      setIsPublishing(false);
    }
  }, [detail, queryClient]);

  return { publish, isPublishing, progress };
}
