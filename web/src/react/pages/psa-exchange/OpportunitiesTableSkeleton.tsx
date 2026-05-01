import Skeleton from '../../ui/Skeleton';
import CardShell from '../../ui/CardShell';

interface OpportunitiesTableSkeletonProps {
  rows?: number;
}

export default function OpportunitiesTableSkeleton({ rows = 5 }: OpportunitiesTableSkeletonProps) {
  return (
    <CardShell>
      <div className="p-4 space-y-3" aria-busy="true">
        <span role="status" aria-live="polite" className="sr-only">
          Loading opportunities
        </span>
        <Skeleton className="h-5 w-1/3" />
        {Array.from({ length: rows }).map((_, i) => (
          <div key={i} data-testid="opportunities-skeleton-row" className="flex items-center gap-4">
            <Skeleton className="h-4 w-1/3" />
            <Skeleton className="h-4 w-16" />
            <Skeleton className="h-4 w-20" />
            <Skeleton className="h-4 w-24 ml-auto" />
          </div>
        ))}
      </div>
    </CardShell>
  );
}
