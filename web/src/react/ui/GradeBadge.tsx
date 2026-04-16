// General-purpose grade badge using grade-color logic and size variants.

interface GradeBadgeProps {
  grader?: string;
  grade: number;
  size?: 'sm' | 'md';
}

function gradeColor(grader: string, grade: number): { bg: string; text: string; border: string } {
  const g = grader.toUpperCase();
  if (g === 'BGS' && grade >= 10) return { bg: 'rgba(0,0,0,0.3)', text: '#e5e7eb', border: 'rgba(255,255,255,0.2)' };
  if (g === 'CGC') return { bg: 'rgba(245,158,11,0.12)', text: 'var(--grade-cgc)', border: 'rgba(245,158,11,0.25)' };
  // Non-10 BGS and all other graders fall through to PSA color tiers
  if (grade >= 10) return { bg: 'rgba(220,38,38,0.12)', text: 'var(--grade-psa10)', border: 'rgba(220,38,38,0.25)' };
  if (grade >= 9) return { bg: 'rgba(37,99,235,0.12)', text: 'var(--grade-psa9)', border: 'rgba(37,99,235,0.25)' };
  if (grade >= 8) return { bg: 'rgba(245,158,11,0.12)', text: 'var(--grade-psa8)', border: 'rgba(245,158,11,0.25)' };
  return { bg: 'rgba(255,255,255,0.05)', text: 'var(--text-muted)', border: 'rgba(255,255,255,0.1)' };
}

export default function GradeBadge({ grader = 'PSA', grade, size = 'sm' }: GradeBadgeProps) {
  const color = gradeColor(grader, grade);
  const label = grader.toUpperCase() === 'PSA' ? `${grade}` : `${grader} ${grade}`;
  const sizeClass = size === 'sm' ? 'text-[10px] px-1.5 py-0.5' : 'text-xs px-2 py-0.5';

  return (
    <span
      className={`inline-flex items-center font-semibold rounded-full tabular-nums shrink-0 ${sizeClass}`}
      style={{ background: color.bg, color: color.text, border: `1px solid ${color.border}` }}
      title={`${grader} ${grade}`}
    >
      {label}
    </span>
  );
}
