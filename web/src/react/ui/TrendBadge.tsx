import { formatTrend } from '../utils/formatters';

interface TrendBadgeProps {
  value: number | null | undefined;
}

export function TrendBadge({ value }: TrendBadgeProps) {
  const t = formatTrend(value);
  if (!t) return null;
  return <span className={`${t.colorClass} ml-1`}>{t.text}</span>;
}

export default TrendBadge;
