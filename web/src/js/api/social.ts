/**
 * Social media and Instagram API methods
 */

import type { SocialPost, SocialPostDetail, InstagramStatus } from '../../types/social';
import type { APIClient } from './client';
import { UPLOAD_TIMEOUT_MS } from './client';

/* ------------------------------------------------------------------ */
/*  Declaration merging — tells TypeScript about the methods we add   */
/* ------------------------------------------------------------------ */

declare module './client' {
  interface APIClient {
    // Social content
    getSocialPosts(status?: string): Promise<SocialPost[]>;
    getSocialPost(id: string): Promise<SocialPostDetail>;
    generateSocialPosts(): Promise<{ status: string }>;
    updateSocialCaption(id: string, caption: string, hashtags: string): Promise<void>;
    deleteSocialPost(id: string): Promise<void>;
    publishSocialPost(id: string): Promise<{ status: string }>;
    uploadSlides(id: string, slides: Blob[]): Promise<{ slides: number }>;

    // Instagram integration
    getInstagramStatus(): Promise<InstagramStatus>;
    connectInstagram(): Promise<{ url: string }>;
    disconnectInstagram(): Promise<void>;
  }
}

/* ------------------------------------------------------------------ */
/*  Prototype implementations                                         */
/* ------------------------------------------------------------------ */

import { APIClient as _APIClient } from './client';
const proto = _APIClient.prototype;

// Social content endpoints
proto.getSocialPosts = async function (this: APIClient, status?: string): Promise<SocialPost[]> {
  const query = status ? `?status=${encodeURIComponent(status)}` : '';
  return this.get<SocialPost[]>(`/social/posts${query}`);
};

proto.getSocialPost = async function (this: APIClient, id: string): Promise<SocialPostDetail> {
  return this.get<SocialPostDetail>(`/social/posts/${id}`);
};

proto.generateSocialPosts = async function (this: APIClient): Promise<{ status: string }> {
  return this.post<{ status: string }>('/social/posts/generate');
};

proto.updateSocialCaption = async function (this: APIClient, id: string, caption: string, hashtags: string): Promise<void> {
  const response = await this.fetchWithRetry(
    `${this.baseURL}/social/posts/${id}/caption`,
    {
      method: 'PATCH',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({ caption, hashtags }),
    }
  );
  await this.expectNoContent(response);
};

proto.deleteSocialPost = async function (this: APIClient, id: string): Promise<void> {
  return this.deleteResource(`/social/posts/${id}`);
};

proto.publishSocialPost = async function (this: APIClient, id: string): Promise<{ status: string }> {
  // Use maxRetries as attempt to skip retry logic — publish is not idempotent
  const response = await this.fetchWithRetry(
    `${this.baseURL}/social/posts/${id}/publish`,
    {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: '{}',
    },
    this.maxRetries,
  );
  return response.json();
};

proto.uploadSlides = async function (this: APIClient, id: string, slides: Blob[]): Promise<{ slides: number }> {
  const formData = new FormData();
  slides.forEach((blob, i) => {
    formData.append(`slide-${i}`, blob, `slide-${i}.jpg`);
  });
  // Do NOT set Content-Type header — browser sets multipart boundary automatically
  const response = await this.fetchWithRetry(
    `${this.baseURL}/social/posts/${id}/upload-slides`,
    {
      method: 'POST',
      body: formData,
    },
    1,
    { timeoutMs: UPLOAD_TIMEOUT_MS },
  );
  return response.json();
};

// Instagram integration endpoints
proto.getInstagramStatus = async function (this: APIClient): Promise<InstagramStatus> {
  return this.get<InstagramStatus>('/instagram/status');
};

proto.connectInstagram = async function (this: APIClient): Promise<{ url: string }> {
  return this.post<{ url: string }>('/instagram/connect');
};

proto.disconnectInstagram = async function (this: APIClient): Promise<void> {
  const response = await this.fetchWithRetry(
    `${this.baseURL}/instagram/disconnect`,
    { method: 'POST' }
  );
  await this.expectNoContent(response);
};
