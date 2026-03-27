import { useState, useCallback, useRef } from 'react';
import { flushSync } from 'react-dom';
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

/**
 * Fetches an external image through our backend proxy and returns a data URL.
 * Data URLs are already inline so html-to-image can embed them in the SVG
 * foreignObject without needing to fetch — avoids CSP connect-src issues.
 */
async function proxyImageToDataUrl(externalUrl: string): Promise<string> {
  const resp = await fetch(`/api/image-proxy?url=${encodeURIComponent(externalUrl)}`);
  if (!resp.ok) {
    console.warn(`Image proxy returned ${resp.status} for ${externalUrl}`);
    return externalUrl;
  }
  const blob = await resp.blob();
  return new Promise<string>((resolve, reject) => {
    const reader = new FileReader();
    reader.onloadend = () => resolve(reader.result as string);
    reader.onerror = reject;
    reader.readAsDataURL(blob);
  });
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

  // Mount the React component synchronously so the DOM is populated before
  // we query for <img> elements and capture with html-to-image.
  const root = createRoot(container);
  flushSync(() => {
    root.render(element);
  });

  // Wait for all images in the container to finish loading (or fail).
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
  // Extra frame for the browser to paint after images load
  await new Promise((resolve) => setTimeout(resolve, 100));

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
      // Pre-fetch card images through backend proxy and convert to data URLs.
      // Data URLs are already inline so html-to-image can embed them directly
      // in the SVG foreignObject without any fetch — avoids CSP issues entirely.
      setProgress('Loading card images...');
      const proxiedCards = await Promise.all(
        detail.cards.map(async (card) => {
          if (!card.frontImageUrl || card.frontImageUrl.startsWith('data:')) return card;
          try {
            const dataUrl = await proxyImageToDataUrl(card.frontImageUrl);
            return { ...card, frontImageUrl: dataUrl };
          } catch (err) {
            console.warn('Image proxy failed for', card.frontImageUrl, err);
            return card;
          }
        }),
      );

      const totalSlides = proxiedCards.length + 1;
      const psa10Count = proxiedCards.filter((c) => c.gradeValue === 10).length;
      const slides: Blob[] = [];

      // Render cover slide
      setProgress('Rendering cover slide...');
      const coverBlob = await renderSlideToJpeg(
        <CoverSlide
          postType={detail.postType}
          coverTitle={detail.coverTitle}
          cardCount={detail.cardCount}
          psa10Count={psa10Count}
          cards={proxiedCards}
          backgroundUrls={detail.backgroundUrls}
        />,
      );
      slides.push(coverBlob);

      // Render each card slide
      for (let i = 0; i < proxiedCards.length; i++) {
        setProgress(`Rendering slide ${i + 2} of ${totalSlides}...`);
        const cardBlob = await renderSlideToJpeg(
          <CardSlide
            card={proxiedCards[i] as PostCardDetail}
            postType={detail.postType}
            slideIndex={i + 1}
            totalSlides={totalSlides}
            backgroundUrl={detail.backgroundUrls?.[i + 1]}
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
