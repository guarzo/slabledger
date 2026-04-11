import { useState, useCallback, useRef } from 'react';
import { useQueryClient } from '@tanstack/react-query';
import { api } from '../../js/api';
import { reportError } from '../../js/errors';
import { queryKeys } from '../queries/queryKeys';
import type { SocialPostDetail } from '../../types/social';

interface UsePublishWithSlidesResult {
  publish: () => Promise<void>;
  isPublishing: boolean;
  progress: string;
}

/**
 * Captures the currently visible [data-slide] element as a JPEG blob.
 * Uses the same html-to-image path as the working download feature.
 */
async function captureVisibleSlide(): Promise<Blob> {
  const { toJpeg } = await import('html-to-image');

  const el = document.querySelector('[data-slide]') as HTMLElement | null;
  if (!el) throw new Error('No [data-slide] element found in DOM');

  // Temporarily force the element (and its parent container) to 1080x1080
  // so html-to-image captures at Instagram's native resolution.
  const parent = el.parentElement;
  const savedParentStyle = parent?.style.cssText ?? '';
  const savedElStyle = el.style.cssText;
  if (parent) {
    parent.style.cssText += ';width:1080px!important;height:1080px!important;max-width:none!important;';
  }
  el.style.cssText += ';width:1080px!important;height:1080px!important;';

  // Let the browser reflow at the new size
  await new Promise((resolve) => setTimeout(resolve, 50));

  const dataUrl = await toJpeg(el, {
    width: 1080,
    height: 1080,
    pixelRatio: 2,
    quality: 0.92,
    backgroundColor: '#0a0e1a',
  });

  // Restore original styles
  if (parent) parent.style.cssText = savedParentStyle;
  el.style.cssText = savedElStyle;

  const [header, base64] = dataUrl.split(',');
  const mime = header.match(/:(.*?);/)?.[1] ?? 'image/jpeg';
  const bytes = atob(base64);
  const buf = new Uint8Array(bytes.length);
  for (let i = 0; i < bytes.length; i++) buf[i] = bytes.charCodeAt(i);
  const blob = new Blob([buf], { type: mime });

  if (blob.size < 50_000) {
    reportError(
      'usePublishWithSlides/slide-size',
      new Error(`slide capture suspiciously small (${blob.size} bytes)`),
    );
  }
  return blob;
}

/** Wait for the next [data-slide] to appear and its images to load. */
async function waitForSlideReady(): Promise<void> {
  // Wait for React to commit the new slide to the DOM
  await new Promise((resolve) => setTimeout(resolve, 150));

  const el = document.querySelector('[data-slide]') as HTMLElement | null;
  if (!el) return;

  const imgs = Array.from(el.querySelectorAll('img'));
  await Promise.all(
    imgs.map((img) =>
      img.complete
        ? Promise.resolve()
        : new Promise<void>((resolve) => {
            img.addEventListener('load', () => resolve(), { once: true });
            img.addEventListener('error', () => resolve(), { once: true });
          }),
    ),
  );
  // Extra frame for paint
  await new Promise((resolve) => setTimeout(resolve, 100));
}

export function usePublishWithSlides(
  detail: SocialPostDetail | undefined,
  setCurrentSlide?: (index: number) => void,
): UsePublishWithSlidesResult {
  const [isPublishing, setIsPublishing] = useState(false);
  const [progress, setProgress] = useState('');
  const isPublishingRef = useRef(false);
  const queryClient = useQueryClient();

  const publish = useCallback(async () => {
    if (!detail || !detail.cards?.length || !setCurrentSlide) return;
    if (isPublishingRef.current) return;

    isPublishingRef.current = true;
    setIsPublishing(true);
    try {
      const totalSlides = detail.cards.length + 1;
      const slides: Blob[] = [];

      // Capture cover slide (index 0)
      setProgress('Rendering cover slide...');
      setCurrentSlide(0);
      await waitForSlideReady();
      slides.push(await captureVisibleSlide());

      // Capture each card slide
      for (let i = 0; i < detail.cards.length; i++) {
        setProgress(`Rendering slide ${i + 2} of ${totalSlides}...`);
        setCurrentSlide(i + 1);
        await waitForSlideReady();
        slides.push(await captureVisibleSlide());
      }

      // Upload slides
      setProgress('Uploading slides...');
      await api.uploadSlides(detail.id, slides);

      // Publish to Instagram
      setProgress('Publishing to Instagram...');
      await api.publishSocialPost(detail.id);

      queryClient.invalidateQueries({ queryKey: queryKeys.social.all });
      setProgress('Published!');
    } catch (error) {
      setProgress('');
      throw error;
    } finally {
      isPublishingRef.current = false;
      setIsPublishing(false);
    }
  }, [detail, setCurrentSlide, queryClient]);

  return { publish, isPublishing, progress };
}
