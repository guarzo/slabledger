import { useEffect, useRef, useState, useCallback } from 'react';
import { createPortal } from 'react-dom';
import { clsx } from 'clsx';
import { ExternalLinkIcon } from './ExternalLinkIcon';

interface LinkDropdownProps {
  links: { label: string; href: string }[];
  size?: 'sm' | 'md';
  align?: 'left' | 'right';
  stopPropagation?: boolean;
}

export function LinkDropdown({ links, size = 'sm', align = 'right', stopPropagation }: LinkDropdownProps) {
  const [open, setOpen] = useState(false);
  const btnRef = useRef<HTMLButtonElement>(null);
  const dropdownRef = useRef<HTMLDivElement>(null);
  const [pos, setPos] = useState({ top: 0, left: 0 });

  const updatePos = useCallback(() => {
    if (!btnRef.current) return;
    const rect = btnRef.current.getBoundingClientRect();
    setPos({
      top: rect.bottom + 4,
      left: align === 'left' ? rect.left : rect.right,
    });
  }, [align]);

  useEffect(() => {
    if (!open) return;
    updatePos();

    function handleClickOutside(e: MouseEvent) {
      const target = e.target as Node;
      if (btnRef.current?.contains(target)) return;
      if (dropdownRef.current?.contains(target)) return;
      setOpen(false);
    }
    function handleScroll() { setOpen(false); }

    document.addEventListener('mousedown', handleClickOutside);
    const scrollParent = btnRef.current?.closest('.overflow-y-auto');
    scrollParent?.addEventListener('scroll', handleScroll);
    return () => {
      document.removeEventListener('mousedown', handleClickOutside);
      scrollParent?.removeEventListener('scroll', handleScroll);
    };
  }, [open, updatePos]);

  const handleToggle = (e: React.MouseEvent) => {
    if (stopPropagation) e.stopPropagation();
    setOpen(prev => !prev);
  };

  const btnSize = size === 'md' ? 'w-6 h-6' : 'w-5 h-5';
  const iconSize = size === 'md' ? 'w-4 h-4' : 'w-3.5 h-3.5';
  const minWidth = size === 'md' ? 'min-w-[140px]' : 'min-w-[120px]';

  return (
    <span className={clsx('shrink-0', size === 'sm' && 'ml-1')}>
      <button
        ref={btnRef}
        type="button"
        onClick={handleToggle}
        onKeyDown={stopPropagation ? (e) => e.stopPropagation() : undefined}
        className={clsx('inline-flex items-center justify-center rounded hover:bg-[var(--surface-2)]/60 text-[var(--text-muted)] hover:text-[var(--brand-400)] transition-colors', btnSize)}
        title="Marketplace links"
        aria-expanded={open}
      >
        <svg className={iconSize} fill="none" viewBox="0 0 24 24" stroke="currentColor" strokeWidth={2} aria-hidden="true">
          <path strokeLinecap="round" strokeLinejoin="round" d="M13.828 10.172a4 4 0 00-5.656 0l-4 4a4 4 0 105.656 5.656l1.102-1.101m-.758-4.899a4 4 0 005.656 0l4-4a4 4 0 00-5.656-5.656l-1.1 1.1" />
        </svg>
      </button>
      {open && createPortal(
        <div
          ref={dropdownRef}
          className={clsx('fixed rounded-lg border border-[var(--surface-2)] bg-[var(--surface-1)] shadow-lg py-1', minWidth)}
          style={{
            top: pos.top,
            left: pos.left,
            zIndex: 9999,
            animation: 'inventory-expand-in 150ms ease-out',
            ...(align === 'right' ? { transform: 'translateX(-100%)' } : undefined),
          }}
        >
          {links.map(link => (
            <a
              key={link.label}
              href={link.href}
              target="_blank"
              rel="noopener noreferrer"
              onClick={stopPropagation ? (e) => e.stopPropagation() : undefined}
              className="flex items-center gap-2 px-3 py-1.5 text-xs text-[var(--text-muted)] hover:text-[var(--brand-400)] hover:bg-[var(--surface-2)]/50 transition-colors"
            >
              <ExternalLinkIcon />
              {link.label}
            </a>
          ))}
        </div>,
        document.body,
      )}
    </span>
  );
}
