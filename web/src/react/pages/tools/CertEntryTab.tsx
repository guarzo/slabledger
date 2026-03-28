import { useEffect, useRef, useState } from 'react';
import { api } from '@/js/api';
import type { CertImportResult } from '@/types/campaigns/core';

export default function CertEntryTab() {
  const [input, setInput] = useState('');
  const [loading, setLoading] = useState(false);
  const [result, setResult] = useState<CertImportResult | null>(null);
  const [error, setError] = useState<string | null>(null);

  const handleImport = async () => {
    const certNumbers = input
      .split('\n')
      .map(s => s.trim())
      .filter(s => s.length > 0);

    if (certNumbers.length === 0) {
      setError('Please enter at least one certificate number');
      return;
    }

    setLoading(true);
    setError(null);
    setResult(null);

    try {
      const res = await api.importCerts(certNumbers);
      setResult(res);
      if (res.imported > 0 || res.alreadyExisted > 0) {
        setInput('');
      }
    } catch (err) {
      setError(err instanceof Error ? err.message : 'Import failed');
    } finally {
      setLoading(false);
    }
  };

  const textareaRef = useRef<HTMLTextAreaElement>(null);
  const cursorRef = useRef<number | null>(null);

  useEffect(() => {
    if (cursorRef.current != null && textareaRef.current) {
      textareaRef.current.selectionStart = textareaRef.current.selectionEnd = cursorRef.current;
      cursorRef.current = null;
    }
  }, [input]);

  const handleKeyDown = (e: React.KeyboardEvent<HTMLTextAreaElement>) => {
    if (e.key === 'Tab') {
      e.preventDefault();
      const start = e.currentTarget.selectionStart;
      const end = e.currentTarget.selectionEnd;
      cursorRef.current = start + 1;
      setInput(input.substring(0, start) + '\n' + input.substring(end));
    }
  };

  return (
    <div className="space-y-4">
      <p className="text-sm text-gray-400">
        Paste PSA certificate numbers (one per line) to import cards directly.
        Existing certs will be flagged for eBay export.
      </p>

      <textarea
        ref={textareaRef}
        value={input}
        onChange={e => setInput(e.target.value)}
        onKeyDown={handleKeyDown}
        placeholder={"12345678\n87654321\n11223344"}
        rows={10}
        className="w-full rounded border border-gray-700 bg-gray-900 p-3 font-mono text-sm text-gray-100 placeholder-gray-600 focus:border-blue-500 focus:outline-none"
        disabled={loading}
      />

      <button
        onClick={handleImport}
        disabled={loading || input.trim().length === 0}
        className="rounded bg-blue-600 px-4 py-2 text-sm font-medium text-white hover:bg-blue-500 disabled:opacity-50"
      >
        {loading ? 'Importing...' : 'Import Certificates'}
      </button>

      {error && (
        <div className="rounded border border-red-700 bg-red-900/30 p-3 text-sm text-red-300">
          {error}
        </div>
      )}

      {result && (
        <div className="space-y-2 rounded border border-gray-700 bg-gray-800 p-4">
          <h3 className="text-sm font-medium text-gray-200">Import Results</h3>
          <div className="grid grid-cols-3 gap-4 text-sm">
            <div>
              <span className="text-green-400">{result.imported}</span>{' '}
              <span className="text-gray-400">imported</span>
            </div>
            <div>
              <span className="text-blue-400">{result.alreadyExisted}</span>{' '}
              <span className="text-gray-400">already existed</span>
            </div>
            <div>
              <span className="text-red-400">{result.failed}</span>{' '}
              <span className="text-gray-400">failed</span>
            </div>
          </div>

          {result.errors.length > 0 && (
            <div className="mt-2 space-y-1">
              <h4 className="text-xs font-medium text-gray-400">Errors:</h4>
              {result.errors.map((e, i) => (
                <div key={i} className="text-xs text-red-400">
                  Cert {e.certNumber}: {e.error}
                </div>
              ))}
            </div>
          )}
        </div>
      )}
    </div>
  );
}
