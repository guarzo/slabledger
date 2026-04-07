import { useState, useRef, useCallback } from 'react';

export function UploadZone({ onFile }: { onFile: (file: File) => void }) {
  const fileRef = useRef<HTMLInputElement>(null);
  const [dragOver, setDragOver] = useState(false);

  const handleDrop = useCallback((e: React.DragEvent) => {
    e.preventDefault();
    setDragOver(false);
    const file = e.dataTransfer.files[0];
    if (file) onFile(file);
  }, [onFile]);

  return (
    <div
      role="button"
      tabIndex={0}
      className={`border-2 border-dashed rounded-xl p-12 text-center cursor-pointer transition-colors ${
        dragOver ? 'border-[var(--brand-500)] bg-[var(--brand-500)]/5' : 'border-[var(--surface-2)] hover:border-[var(--brand-500)]/50'
      }`}
      onDragOver={e => { e.preventDefault(); setDragOver(true); }}
      onDragLeave={() => setDragOver(false)}
      onDrop={handleDrop}
      onClick={() => fileRef.current?.click()}
      onKeyDown={e => { if (e.key === 'Enter' || e.key === ' ') { e.preventDefault(); fileRef.current?.click(); } }}
    >
      <svg width="48" height="48" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="1.5" strokeLinecap="round" strokeLinejoin="round" className="mx-auto mb-4 text-[var(--text-muted)]">
        <path d="M21 15v4a2 2 0 01-2 2H5a2 2 0 01-2-2v-4" />
        <polyline points="17 8 12 3 7 8" />
        <line x1="12" y1="3" x2="12" y2="15" />
      </svg>
      <div className="text-sm font-medium text-[var(--text)]">Drop a CSV here or click to browse</div>
      <div className="text-xs text-[var(--text-muted)] mt-1">Supports Shopify product CSV or eBay graded batch export</div>
      <input ref={fileRef} type="file" accept=".csv" className="hidden" onChange={e => {
        const file = e.target.files?.[0];
        if (file) onFile(file);
        e.target.value = '';
      }} />
    </div>
  );
}
