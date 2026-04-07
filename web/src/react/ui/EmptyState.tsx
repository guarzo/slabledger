import Button from './Button';

interface EmptyStateProps {
  icon: string;
  title: string;
  description: string;
  action?: {
    label: string;
    onClick: () => void;
  };
  /** Reduce vertical padding for inline/section use */
  compact?: boolean;
  /** Onboarding steps shown as a numbered list */
  steps?: string[];
}

export default function EmptyState({ icon, title, description, action, compact, steps }: EmptyStateProps) {
  return (
    <div className={`flex flex-col items-center justify-center ${compact ? 'py-6' : 'py-16'} text-center`}>
      <div className={`${compact ? 'text-3xl mb-2' : 'text-5xl mb-4'}`}>{icon}</div>
      <h3 className={`${compact ? 'text-sm' : 'text-lg'} font-semibold text-[var(--text)] mb-2`}>{title}</h3>
      <p className="text-sm text-[var(--text-muted)] max-w-md mb-4">{description}</p>
      {steps && steps.length > 0 && (
        <ol className="text-left text-sm text-[var(--text-muted)] mb-4 space-y-1 max-w-xs">
          {steps.map((step, i) => (
            <li key={i} className="flex gap-2">
              <span className="text-[var(--brand-400)] font-semibold shrink-0">{i + 1}.</span>
              <span>{step}</span>
            </li>
          ))}
        </ol>
      )}
      {action && (
        <Button variant="primary" onClick={action.onClick}>
          {action.label}
        </Button>
      )}
    </div>
  );
}
