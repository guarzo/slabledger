import { useState, useCallback, useRef } from 'react';
import { flushSync } from 'react-dom';
import { createRoot } from 'react-dom/client';
import html2canvas from 'html2canvas';
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
 * Fetches an external image through our backend proxy and returns a blob URL.
 * Blob URLs are small references that html2canvas can render directly from
 * the DOM without inflating the HTML size.
 */
async function proxyImageToBlobUrl(externalUrl: string): Promise<string> {
  const resp = await fetch(`/api/image-proxy?url=${encodeURIComponent(externalUrl)}`);
  if (!resp.ok) {
    console.warn(`[publish] proxy FAILED ${resp.status} for ${externalUrl}`);
    return externalUrl;
  }
  const blob = await resp.blob();
  return URL.createObjectURL(blob);
}

async function renderSlideToJpeg(
  element: React.ReactElement,
): Promise<Blob> {
  // Create offscreen container at Instagram's native 1080x1080
  const container = document.createElement('div');
  container.style.cssText =
    'position:fixed;left:-9999px;top:0;width:1080px;height:1080px;overflow:hidden;';
  document.body.appendChild(container);

  // Mount the React component synchronously so the DOM is populated before
  // we query for <img> elements and capture with html2canvas.
  const root = createRoot(container);
  flushSync(() => {
    root.render(element);
  });

  // Wait for all images in the container to finish loading (or fail).
  const imgs = Array.from(container.querySelectorAll('img'));
  await Promise.all(
    imgs.map(
      (img) =>
        img.complete
          ? Promise.resolve()
          : new Promise<void>((resolve) => {
              img.addEventListener('load', () => resolve(), { once: true });
              img.addEventListener('error', () => {
                console.warn(`[publish] img FAILED to load: ${img.src.substring(0, 80)}`);
                resolve();
              }, { once: true });
            }),
    ),
  );
  // Extra frame for the browser to paint after images load
  await new Promise((resolve) => setTimeout(resolve, 100));

  try {
    // html2canvas rasterizes directly to a canvas element — no SVG foreignObject
    // intermediate step, so there's no data URL size limit that causes blank output.
    const canvas = await html2canvas(container, {
      width: 1080,
      height: 1080,
      scale: 2,
      backgroundColor: '#0a0e1a',
      useCORS: true,
      allowTaint: false,
      logging: false,
    });

    return new Promise<Blob>((resolve, reject) => {
      canvas.toBlob(
        (blob) => {
          if (blob) {
            console.warn(`[publish] slide blob: ${blob.size} bytes`);
            resolve(blob);
          } else {
            reject(new Error('canvas.toBlob returned null'));
          }
        },
        'image/jpeg',
        0.92,
      );
    });
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
    const blobUrls: string[] = [];
    try {
      // Pre-fetch card images through backend proxy to get same-origin blob URLs.
      // html2canvas renders directly from the DOM so blob URLs work fine.
      setProgress('Loading card images...');
      const proxiedCards = await Promise.all(
        detail.cards.map(async (card) => {
          if (!card.frontImageUrl || card.frontImageUrl.startsWith('blob:')) return card;
          try {
            const blobUrl = await proxyImageToBlobUrl(card.frontImageUrl);
            if (blobUrl !== card.frontImageUrl) blobUrls.push(blobUrl);
            return { ...card, frontImageUrl: blobUrl };
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
      blobUrls.forEach((u) => URL.revokeObjectURL(u));
      isPublishingRef.current = false;
      setIsPublishing(false);
    }
  }, [detail, queryClient]);

  return { publish, isPublishing, progress };
}
