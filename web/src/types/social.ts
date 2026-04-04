export type PostType = 'new_arrivals' | 'price_movers' | 'hot_deals';
export type PostStatus = 'draft' | 'publishing' | 'published' | 'failed' | 'approved' | 'rejected';

export interface SocialPost {
  id: string;
  postType: PostType;
  status: PostStatus;
  caption: string;
  hashtags: string;
  coverTitle: string;
  cardCount: number;
  instagramPostId?: string;
  errorMessage?: string;
  slideUrls?: string[];
  backgroundUrls?: string[];
  createdAt: string;
  updatedAt: string;
}

export interface PostCardDetail {
  purchaseId: string;
  slideOrder: number;
  cardName: string;
  setName: string;
  cardNumber: string;
  gradeValue: number;
  grader: string;
  certNumber: string;
  frontImageUrl: string;
  askingPriceCents: number;
  clValueCents: number;
  trend30d: number;
}

export interface SocialPostDetail extends SocialPost {
  cards?: PostCardDetail[];
}

export interface InstagramStatus {
  connected: boolean;
  username?: string;
  expiresAt?: string;
  connectedAt?: string;
}

export interface PostMetricsSnapshot {
  id: number;
  postId: string;
  impressions: number;
  reach: number;
  likes: number;
  comments: number;
  saves: number;
  shares: number;
  polledAt: string;
}

export interface MetricsSummary {
  postId: string;
  postType: PostType;
  coverTitle: string;
  impressions: number;
  reach: number;
  likes: number;
  comments: number;
  saves: number;
  shares: number;
  publishedAt: string;
}
