import { Link } from 'react-router-dom';

interface BreadcrumbItem {
  label: string;
  href?: string;
}

interface BreadcrumbProps {
  items: BreadcrumbItem[];
}

export default function Breadcrumb({ items }: BreadcrumbProps) {
  return (
    <nav aria-label="Breadcrumb" className="mb-4">
      <ol className="flex items-center gap-2 text-[13px]">
        {items.map((item, i) => {
          const isLast = i === items.length - 1;
          return (
            <li key={i} className="flex items-center gap-2">
              {i > 0 && (
                <span className="text-[var(--text-muted)]" aria-hidden="true">/</span>
              )}
              {item.href && !isLast ? (
                <Link
                  to={item.href}
                  className="text-[var(--text-muted)] hover:text-[var(--text)] hover:underline transition-colors"
                >
                  {item.label}
                </Link>
              ) : (
                <span
                  className="text-[var(--text)] font-medium"
                  aria-current={isLast ? 'page' : undefined}
                >
                  {item.label}
                </span>
              )}
            </li>
          );
        })}
      </ol>
    </nav>
  );
}
