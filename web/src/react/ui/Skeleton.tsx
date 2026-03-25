import { cva, type VariantProps } from 'class-variance-authority';
import { clsx } from 'clsx';

const skeletonVariants = cva('animate-pulse rounded bg-[var(--surface-2)]', {
  variants: {
    variant: {
      text: 'h-4 w-full',
      heading: 'h-6 w-3/4',
      rect: 'h-24 w-full',
      circle: 'rounded-full',
      card: 'h-48 w-full rounded-xl',
    },
  },
  defaultVariants: { variant: 'text' },
});

export interface SkeletonProps
  extends React.HTMLAttributes<HTMLDivElement>,
    VariantProps<typeof skeletonVariants> {}

export function Skeleton({ variant, className, ...props }: SkeletonProps) {
  return (
    <div
      className={clsx(skeletonVariants({ variant }), className)}
      aria-hidden="true"
      {...props}
    />
  );
}

export default Skeleton;
