import { useState, useEffect, useCallback } from 'react';
import { api, isAPIError } from '../../../../js/api';
import { useToast } from '../../../contexts/ToastContext';

const DH_URL_PATTERN = /doubleholo\.com\/card\/(\d+)/;

interface FixDHMatchDialogProps {
  purchaseId: string;
  cardName: string;
  certNumber?: string;
  currentDHCardId?: number;
  onClose: () => void;
  onSaved: () => void;
}

export default function FixDHMatchDialog({
  purchaseId,
  cardName,
  certNumber,
  currentDHCardId,
  onClose,
  onSaved,
}: FixDHMatchDialogProps) {
  const [url, setUrl] = useState('');
  const [saving, setSaving] = useState(false);
  const toast = useToast();

  useEffect(() => {
    const handleKeyDown = (e: KeyboardEvent) => {
      if (e.key === 'Escape' && !saving) onClose();
    };
    document.addEventListener('keydown', handleKeyDown);
    return () => document.removeEventListener('keydown', handleKeyDown);
  }, [onClose, saving]);

  const parsedId = DH_URL_PATTERN.exec(url.trim())?.[1];
  const isValid = !!parsedId && parsedId !== String(currentDHCardId ?? '');

  const handleSave = useCallback(async () => {
    if (!isValid) return;
    setSaving(true);
    try {
      const res = await api.fixDHMatch({ purchaseId, dhUrl: url.trim() });
      toast.success(`DH match updated (card #${res.dhCardId})`);
      onSaved();
      onClose();
    } catch (e) {
      const msg = isAPIError(e) ? e.message : 'Failed to update DH match';
      toast.error(msg);
    } finally {
      setSaving(false);
    }
  }, [isValid, purchaseId, url, toast, onSaved, onClose]);

  return (
    <div
      className="fixed inset-0 z-50 flex items-center justify-center bg-black/50"
      onClick={() => { if (!saving) onClose(); }}
    >
      <div
        role="dialog"
        aria-modal="true"
        aria-labelledby="fix-dh-match-title"
        tabIndex={-1}
        className="bg-[var(--surface-1)] rounded-lg shadow-xl p-6 max-w-md w-full mx-4"
        onClick={(e) => e.stopPropagation()}
      >
        <h3 id="fix-dh-match-title" className="text-lg font-semibold mb-1">Fix DH Match</h3>
        <p className="text-xs text-[var(--text-muted)] mb-4">
          Paste the correct DoubleHolo product URL. The current match will be replaced and DH will be
          taught the correction. Any listing under the old card ID stays on DH — clean up manually there.
        </p>

        <div className="space-y-3 text-sm">
          <div>
            <div className="text-[var(--text-muted)] mb-1">Card</div>
            <div className="font-medium truncate">{cardName}</div>
            <div className="text-xs text-[var(--text-muted)]">
              {certNumber && <>Cert {certNumber}</>}
              {certNumber && currentDHCardId ? ' · ' : ''}
              {!!currentDHCardId && <>Current DH card: {currentDHCardId}</>}
            </div>
          </div>

          <div>
            <label htmlFor="fix-dh-match-url" className="block text-[var(--text-muted)] mb-1">
              DH product URL
            </label>
            <input
              id="fix-dh-match-url"
              type="url"
              value={url}
              onChange={(e) => setUrl(e.target.value)}
              placeholder="https://doubleholo.com/card/123456/..."
              className="w-full px-3 py-2 rounded bg-[var(--surface-2)] border border-[var(--surface-2)] text-[var(--text)] placeholder:text-[var(--text-muted)]"
              autoFocus
              disabled={saving}
            />
            {url.trim() && !parsedId && (
              <div className="mt-1 text-xs text-[var(--danger)]">
                Expected format: doubleholo.com/card/&#123;id&#125;/...
              </div>
            )}
            {parsedId && parsedId === String(currentDHCardId ?? '') && (
              <div className="mt-1 text-xs text-[var(--warning)]">
                That&apos;s the current match — paste a different URL.
              </div>
            )}
            {parsedId && parsedId !== String(currentDHCardId ?? '') && (
              <div className="mt-1 text-xs text-[var(--text-muted)]">
                New DH card ID: <span className="font-medium text-[var(--text)]">{parsedId}</span>
              </div>
            )}
          </div>
        </div>

        <div className="flex justify-end gap-2 mt-6">
          <button
            type="button"
            onClick={onClose}
            disabled={saving}
            className="px-4 py-2 text-sm rounded bg-[var(--surface-2)] hover:bg-[var(--surface-3)] transition-colors disabled:opacity-50"
          >
            Cancel
          </button>
          <button
            type="button"
            onClick={handleSave}
            disabled={saving || !isValid}
            className="px-4 py-2 text-sm rounded bg-[var(--accent)] text-white hover:opacity-90 transition-opacity disabled:opacity-50"
          >
            {saving ? 'Updating…' : 'Update Match'}
          </button>
        </div>
      </div>
    </div>
  );
}
