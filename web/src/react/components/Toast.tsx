import { Toast as RadixToast } from 'radix-ui';

export interface ToastProps {
  type: 'success' | 'error' | 'info' | 'warning';
  message: string;
  open: boolean;
  onOpenChange: (open: boolean) => void;
  duration?: number;
}

const icons: Record<ToastProps['type'], string> = {
  success: '\u2713',
  error: '\u2715',
  info: '\u2139',
  warning: '\u26A0',
};

const colorClasses: Record<ToastProps['type'], string> = {
  success: 'bg-[var(--success)] border-[var(--success-border)] text-white',
  error: 'bg-[var(--danger)] border-[var(--danger-border)] text-white',
  info: 'bg-[var(--info)] border-[var(--info-border)] text-white',
  warning: 'bg-[var(--warning)] border-[var(--warning-border)] text-white',
};

export function Toast({ type, message, open, onOpenChange, duration = 5000 }: ToastProps) {
  return (
    <RadixToast.Root open={open} onOpenChange={onOpenChange} duration={duration}
      className={`flex items-center gap-3 p-4 rounded-lg shadow-lg border min-w-[300px] max-w-md
        data-[state=open]:animate-[slide-in_150ms_ease-out]
        data-[state=closed]:animate-[fadeOut_100ms_ease-in]
        data-[swipe=move]:translate-x-[var(--radix-toast-swipe-move-x)]
        data-[swipe=cancel]:translate-x-0 data-[swipe=cancel]:transition-transform
        data-[swipe=end]:animate-[swipe-out_100ms_ease-out]
        ${colorClasses[type]}`}>
      <span className="text-xl flex-shrink-0" aria-hidden="true">{icons[type]}</span>
      <RadixToast.Description className="flex-1 text-sm font-medium">{message}</RadixToast.Description>
      <RadixToast.Close className="flex-shrink-0 hover:opacity-80 transition-opacity"
                        aria-label="Close notification">
        <svg className="w-4 h-4" fill="none" stroke="currentColor" viewBox="0 0 24 24" aria-hidden="true" focusable="false">
          <path strokeLinecap="round" strokeLinejoin="round" strokeWidth={2} d="M6 18L18 6M6 6l12 12" />
        </svg>
      </RadixToast.Close>
    </RadixToast.Root>
  );
}

export function ToastViewport() {
  return (
    <RadixToast.Viewport
      className="fixed bottom-4 right-4 z-50 flex flex-col gap-2 w-[390px] max-w-[100vw] outline-none"
    />
  );
}
