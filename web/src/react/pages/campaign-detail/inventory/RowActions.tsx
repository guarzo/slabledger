import { useState } from 'react';
import { DropdownMenu } from 'radix-ui';
import { Button, ConfirmDialog } from '../../../ui';
import type { ResolvedAction } from './rowActions';

interface RowActionsProps {
  primary: ResolvedAction | null;
  /** Sell is the loud button when no contextual primary exists. */
  fallbackPrimary: ResolvedAction;
  overflow: ResolvedAction[];
  /** 'desktop' = primary as small Button, overflow as dot-menu. 'mobile' same shape, larger hit targets. */
  variant?: 'desktop' | 'mobile';
}

export default function RowActions({ primary, fallbackPrimary, overflow, variant = 'desktop' }: RowActionsProps) {
  const loud = primary ?? fallbackPrimary;
  const [pendingConfirm, setPendingConfirm] = useState<ResolvedAction | null>(null);

  // Loud button is brand-tinted when contextual, neutral primary when fallback Sell.
  const loudVariant = primary ? 'primary' : 'success';

  const sizeClass = variant === 'mobile' ? 'min-w-[64px]' : '';

  const triggerAction = (action: ResolvedAction) => {
    if (action.confirm) {
      setPendingConfirm(action);
    } else {
      action.onSelect();
    }
  };

  return (
    <div className="flex items-center gap-1.5" onClick={e => e.stopPropagation()}>
      <Button
        variant={loudVariant}
        size="sm"
        onClick={() => triggerAction(loud)}
        disabled={loud.disabled}
        aria-label={loud.label}
        className={sizeClass}
      >
        {loud.label}
      </Button>

      {overflow.length > 0 && (
        <DropdownMenu.Root>
          <DropdownMenu.Trigger asChild>
            <button
              type="button"
              onClick={e => e.stopPropagation()}
              onKeyDown={e => e.stopPropagation()}
              className="p-1.5 rounded text-[var(--text-muted)] hover:text-[var(--text)] hover:bg-[rgba(255,255,255,0.04)] transition-colors"
              aria-label="More actions"
            >
              <svg width="14" height="14" viewBox="0 0 24 24" fill="currentColor" aria-hidden="true">
                <circle cx="12" cy="5" r="2" />
                <circle cx="12" cy="12" r="2" />
                <circle cx="12" cy="19" r="2" />
              </svg>
            </button>
          </DropdownMenu.Trigger>
          <DropdownMenu.Portal>
            <DropdownMenu.Content
              align="end"
              sideOffset={4}
              className="w-48 py-1 bg-[var(--surface-1)] border border-[var(--surface-2)] rounded-lg shadow-lg z-50 data-[state=open]:animate-[fadeIn_150ms_ease-out]"
            >
              {overflow.map((action, idx) => {
                const isDanger = action.confirm?.variant === 'danger';
                const itemClass = isDanger
                  ? 'px-3 py-2 text-sm outline-none cursor-default text-[var(--danger)] hover:bg-[var(--danger-bg)] hover:text-[var(--danger)]'
                  : 'px-3 py-2 text-sm outline-none cursor-default text-[var(--text-muted)] hover:bg-[rgba(255,255,255,0.04)] hover:text-[var(--text)]';
                const showSeparator = isDanger && idx > 0 && overflow[idx - 1]?.confirm?.variant !== 'danger';
                return (
                  <div key={action.key}>
                    {showSeparator && <DropdownMenu.Separator className="my-1 h-px bg-[var(--surface-2)]" />}
                    <DropdownMenu.Item
                      disabled={action.disabled}
                      onSelect={() => triggerAction(action)}
                      className={itemClass}
                    >
                      {action.label}
                    </DropdownMenu.Item>
                  </div>
                );
              })}
            </DropdownMenu.Content>
          </DropdownMenu.Portal>
        </DropdownMenu.Root>
      )}

      {pendingConfirm?.confirm && (
        <ConfirmDialog
          open
          title={pendingConfirm.confirm.title}
          message={pendingConfirm.confirm.message}
          confirmLabel={pendingConfirm.confirm.confirmLabel}
          variant={pendingConfirm.confirm.variant ?? 'danger'}
          onConfirm={() => {
            const action = pendingConfirm;
            setPendingConfirm(null);
            action.onSelect();
          }}
          onCancel={() => setPendingConfirm(null)}
        />
      )}
    </div>
  );
}
