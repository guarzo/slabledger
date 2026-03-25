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
  buyCostCents: number;
  medianCents: number;
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
