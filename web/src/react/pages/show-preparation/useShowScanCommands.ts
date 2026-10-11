import { useEffect, useRef, useState } from 'react';
import { useQueryClient } from '@tanstack/react-query';
import type { ScanResult } from '../../../types/showprep';
import { showPrepAPI } from '../../../js/api/showprep';
import { isScanCommand } from '../../../js/api/showScanValidation';
import { showPrepKeys } from '../../queries/showPrepKeys';
import { AUTH_CONTEXT_RESET_EVENT } from '../../utils/authContextEvents';
import { readPendingScans, writePendingScans, type PendingShowScan } from './showScanRecovery';

export interface ShowScanActivity extends PendingShowScan {
  state: 'queued' | 'sending' | 'uncertain' | 'recovery' | 'settled';
  result?: ScanResult;
  error?: string;
}
export function useShowScanCommands(listId: string, scope: string) {
  const qc = useQueryClient();
  const [activity, setActivity] = useState<ShowScanActivity[]>([]);
  const [error, setError] = useState('');
  const [ready, setReady] = useState(false);
  const rows = useRef<ShowScanActivity[]>([]);
  const context = useRef({ listId, scope });
  context.current = { listId, scope };
  const mounted = useRef(false);
  const epoch = useRef(0);
  const running = useRef<AbortController | null>(null);
  const blocked = useRef(false);
  function publish() { setActivity([...rows.current]); }
  function persist(next: ShowScanActivity[]) {
    writePendingScans(context.current.scope, next.filter(row => row.state !== 'settled').map(row => ({ listId: row.listId, command: row.command })));
  }
  useEffect(() => {
    mounted.current = true;
    blocked.current = false;
    try {
      rows.current = readPendingScans(scope).map(row => ({ ...row, state: 'recovery' }));
      setError(''); setReady(!!scope);
    } catch (err) {
      rows.current = []; blocked.current = true; setReady(false); setError((err as Error).message);
    }
    publish();
    const reset = () => {
      epoch.current++; running.current?.abort(); running.current = null;
      blocked.current = true; rows.current = []; publish(); setReady(false); setError('Authentication reset. Sign in again before scanning.');
    };
    window.addEventListener(AUTH_CONTEXT_RESET_EVENT, reset);
    return () => {
      mounted.current = false;
      // This is a request-lifetime counter, not a DOM ref captured by the effect.
      // eslint-disable-next-line react-hooks/exhaustive-deps
      epoch.current++; running.current?.abort(); running.current = null;
      window.removeEventListener(AUTH_CONTEXT_RESET_EVENT, reset);
    };
  }, [scope]);
  useEffect(() => {
    let changed = false;
    for (const row of rows.current) if (row.state === 'queued' && row.listId !== listId) { row.state = 'recovery'; changed = true; }
    if (changed) publish();
  }, [listId]);

  async function pump(explicitCommandId?: string) {
    if (!mounted.current || blocked.current || running.current || !context.current.scope) return;
    const row = rows.current.find(item => item.state !== 'settled');
    if (!row || row.state !== 'queued') return;
    if (row.listId !== context.current.listId && row.command.commandId !== explicitCommandId) { row.state = 'recovery'; publish(); return; }
    const fence = epoch.current;
    const controller = new AbortController(); running.current = controller;
    row.state = 'sending'; publish();
    try {
      const result = await showPrepAPI.scanConfirm(row.listId, row.command, { signal: controller.signal });
      if (!mounted.current || epoch.current !== fence) return;
      row.result = result;
      const next = rows.current.map(item => item === row ? { ...row, state: 'settled' as const } : item);
      persist(next);
      row.state = 'settled'; row.error = undefined;
      rows.current = rows.current.filter(item => item.state !== 'settled' || rows.current.filter(candidate => candidate.state === 'settled').slice(-20).includes(item));
      void qc.invalidateQueries({ queryKey: showPrepKeys.detail(row.listId) });
      void qc.invalidateQueries({ queryKey: showPrepKeys.lists });
    } catch (err) {
      if (!mounted.current || epoch.current !== fence) return;
      row.state = 'uncertain'; row.result = undefined;
      row.error = err instanceof Error ? err.message : 'Scan response unavailable';
    } finally {
      if (mounted.current && epoch.current === fence) {
        running.current = null; publish();
        // The pump checks the next row's destination/state; old recovery stays explicit.
        void pump();
      }
    }
  }
  function capture(cert: string, generation: number): boolean {
    if (!ready || blocked.current || !context.current.scope) return false;
    if (rows.current.filter(row => row.state !== 'settled').length >= 200) {
      setError('200 unsettled scans. Recover or dismiss pending scans before scanning more.'); return false;
    }
    const command = Object.freeze({ commandId: crypto.randomUUID(), certNumber: cert.trim(), withdrawalGeneration: generation });
    if (!isScanCommand(command)) { setError('Invalid cert or show generation. Cert must be nonempty and at most 128 UTF-8 bytes.'); return false; }
    const row: ShowScanActivity = { listId: context.current.listId, command, state: 'queued' };
    try { persist([...rows.current, row]); }
    catch (err) { setError((err as Error).message); return false; }
    rows.current.push(row); setError(''); publish(); void pump(); return true;
  }
  function retry(commandId: string) {
    if (running.current || blocked.current) return;
    const row = rows.current.find(item => item.command.commandId === commandId);
    if (!row || !['uncertain', 'recovery'].includes(row.state) || rows.current.find(item => item.state !== 'settled') !== row) return;
    // Explicit recovery may target an old show; keep the destination immutable.
    row.state = 'queued'; void pump(commandId); publish();
  }
  function dismiss(commandId: string) {
    const row = rows.current.find(item => item.command.commandId === commandId);
    if (!row || row.state === 'sending') return;
    const next = rows.current.filter(item => item !== row);
    try { persist(next); rows.current = next; setError(''); publish(); void pump(); }
    catch (err) { setError((err as Error).message); }
  }
  return { activity, error, ready, capture, retry, dismiss,
    pending: activity.filter(row => row.state !== 'settled').length,
    sending: activity.some(row => row.state === 'sending') };
}
