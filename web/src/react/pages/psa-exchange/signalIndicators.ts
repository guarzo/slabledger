// Thresholds mirror daysBucketClass / velocityBucketClass / confidenceColorClass
// in utils.ts. Keep them in sync — both sets feed the same Signal cell visuals.

export type DaysTier = 'fast' | 'medium' | 'slow';
export type ConfidenceTier = 'high' | 'medium' | 'low';
export type VelocityTier = 1 | 2 | 3;

export function daysTier(days: number): DaysTier {
  if (!Number.isFinite(days)) return 'slow';
  if (days <= 6) return 'fast';
  if (days <= 15) return 'medium';
  return 'slow';
}

export function velocityTier(velMonth: number): VelocityTier {
  if (velMonth >= 10) return 3;
  if (velMonth >= 3) return 2;
  return 1;
}

export function confidenceTier(conf: number): ConfidenceTier {
  if (conf >= 7) return 'high';
  if (conf >= 5) return 'medium';
  return 'low';
}
