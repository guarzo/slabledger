import { useEffect, useRef, useState } from 'react';

interface CopyableCertProps {
  certNumber: string;
  children?: React.ReactNode;
}

export default function CopyableCert({ certNumber, children }: CopyableCertProps) {
  const [copied, setCopied] = useState(false);
  const timer = useRef<ReturnType<typeof setTimeout> | null>(null);

  useEffect(() => () => {
    if (timer.current) clearTimeout(timer.current);
  }, []);

  if (!certNumber) return null;

  const handleClick = async (e: React.MouseEvent) => {
    e.stopPropagation();
    try {
      await navigator.clipboard.writeText(certNumber);
      setCopied(true);
      if (timer.current) clearTimeout(timer.current);
      timer.current = setTimeout(() => setCopied(false), 1000);
    } catch {
      // Clipboard rejection is rare and non-destructive; no flash signals failure.
    }
  };

  return (
    <button
      type="button"
      onClick={handleClick}
      title="Copy cert number"
      aria-label={`Copy cert number ${certNumber}`}
      className={`inline cursor-pointer bg-transparent border-0 p-0 font-inherit text-inherit hover:text-[var(--text)] hover:underline ${
        copied ? 'text-[var(--success,#16a34a)]' : ''
      }`}
    >
      {copied ? 'Copied ✓' : (children ?? certNumber)}
    </button>
  );
}
