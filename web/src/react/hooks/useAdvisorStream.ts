import { useState, useCallback, useRef, useEffect } from 'react';
import { type ScoreCard } from '../../types/scoring';

interface StreamEvent {
  type: 'delta' | 'tool_start' | 'tool_result' | 'score' | 'done' | 'error';
  content?: string;
  toolName?: string;
}

interface UseAdvisorStreamResult {
  content: string;
  isStreaming: boolean;
  error: string | null;
  toolStatus: string | null;
  scoreCard: ScoreCard | null;
  run: (endpoint: string, body?: Record<string, unknown>) => Promise<void>;
  reset: () => void;
}

// Timeout for the initial SSE connection (not the full stream duration).
// Reasoning models may take 60-90s before streaming begins.
const CONNECT_TIMEOUT_MS = 120_000;

/**
 * Hook for consuming SSE streams from the advisor API endpoints.
 * Accumulates markdown content via a ref and flushes to state per network chunk
 * to minimize React re-renders during streaming.
 */
export function useAdvisorStream(): UseAdvisorStreamResult {
  const [content, setContent] = useState('');
  const [isStreaming, setIsStreaming] = useState(false);
  const [error, setError] = useState<string | null>(null);
  const [toolStatus, setToolStatus] = useState<string | null>(null);
  const [scoreCard, setScoreCard] = useState<ScoreCard | null>(null);
  const abortRef = useRef<AbortController | null>(null);
  const contentRef = useRef('');

  // Abort on unmount to prevent state updates on unmounted component.
  useEffect(() => {
    return () => {
      abortRef.current?.abort();
    };
  }, []);

  const reset = useCallback(() => {
    if (abortRef.current) {
      abortRef.current.abort();
      abortRef.current = null;
    }
    contentRef.current = '';
    setContent('');
    setIsStreaming(false);
    setError(null);
    setToolStatus(null);
    setScoreCard(null);
  }, []);

  const run = useCallback(async (endpoint: string, body?: Record<string, unknown>) => {
    if (abortRef.current) {
      abortRef.current.abort();
    }

    const controller = new AbortController();
    abortRef.current = controller;

    contentRef.current = '';
    setContent('');
    setIsStreaming(true);
    setError(null);
    setToolStatus(null);
    setScoreCard(null);

    // Apply a connection timeout — once streaming starts, the abort is only manual.
    const connectTimeout = setTimeout(() => {
      if (!firstEventReceived) {
        controller.abort(new Error('Connection timeout'));
      }
    }, CONNECT_TIMEOUT_MS);

    let firstEventReceived = false;

    try {
      const url = endpoint.startsWith('/') ? endpoint : `/api/advisor/${endpoint}`;
      const response = await fetch(url, {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: body ? JSON.stringify(body) : '{}',
        credentials: 'include',
        signal: controller.signal,
      });

      if (!response.ok) {
        clearTimeout(connectTimeout);
        const text = await response.text();
        throw new Error(text || `HTTP ${response.status}`);
      }

      const reader = response.body?.getReader();
      if (!reader) {
        clearTimeout(connectTimeout);
        throw new Error('No response body');
      }

      const decoder = new TextDecoder();
      let buffer = '';

      while (true) {
        const { done, value } = await reader.read();
        if (done) break;

        buffer += decoder.decode(value, { stream: true });
        const lines = buffer.split('\n');
        buffer = lines.pop() ?? '';

        // Accumulate deltas into ref, then flush once per network chunk.
        let contentChanged = false;
        for (const line of lines) {
          if (!line.startsWith('data: ')) continue;
          const data = line.slice(6);
          if (data === '[DONE]') continue;

          try {
            const event: StreamEvent = JSON.parse(data);

            // Clear the connect timeout on first successfully parsed event.
            if (!firstEventReceived) {
              firstEventReceived = true;
              clearTimeout(connectTimeout);
            }

            switch (event.type) {
              case 'delta':
                if (event.content) {
                  contentRef.current += event.content;
                  contentChanged = true;
                }
                break;
              case 'tool_start':
                setToolStatus(formatToolName(event.toolName ?? ''));
                break;
              case 'tool_result':
                setToolStatus(null);
                break;
              case 'error':
                setError(event.content ?? 'Unknown error');
                break;
              case 'score':
                if (event.content) {
                  try {
                    setScoreCard(JSON.parse(event.content) as ScoreCard);
                  } catch (e) {
                    console.warn('Failed to parse score event:', event.content, e);
                  }
                }
                break;
              case 'done':
                // Some endpoints send final data in the done event content
                if (event.content) {
                  contentRef.current = event.content;
                  contentChanged = true;
                }
                break;
            }
          } catch (parseErr) {
            console.warn('Failed to parse SSE event:', data, parseErr);
          }
        }

        // Single state update per network chunk instead of per token.
        if (contentChanged) {
          setContent(contentRef.current);
        }
      }
    } catch (err) {
      clearTimeout(connectTimeout);
      if (err instanceof DOMException && err.name === 'AbortError') return;
      setError(err instanceof Error ? err.message : 'Stream failed');
    } finally {
      setIsStreaming(false);
      setToolStatus(null);
      abortRef.current = null;
    }
  }, []);

  return { content, isStreaming, error, toolStatus, scoreCard, run, reset };
}

/** Converts tool names like "get_campaign_pnl" to "Fetching campaign P&L..." */
function formatToolName(name: string): string {
  const map: Record<string, string> = {
    list_campaigns: 'Loading campaigns',
    get_campaign_pnl: 'Fetching campaign P&L',
    get_pnl_by_channel: 'Fetching channel breakdown',
    get_campaign_tuning: 'Analyzing campaign tuning',
    get_inventory_aging: 'Reviewing inventory',
    get_global_inventory: 'Reviewing all inventory',
    get_sell_sheet: 'Building sell sheet',
    get_portfolio_health: 'Checking portfolio health',
    get_portfolio_insights: 'Analyzing portfolio segments',
    get_credit_summary: 'Checking credit health',
    get_weekly_review: 'Reviewing weekly performance',
    get_capital_timeline: 'Loading capital timeline',
    get_expected_values: 'Computing expected values',
    get_deslab_candidates: 'Analyzing deslab candidates',
    get_campaign_suggestions: 'Generating suggestions',
    run_projection: 'Running Monte Carlo simulation',
    get_channel_velocity: 'Checking channel velocity',
    get_cert_lookup: 'Looking up cert details',
    evaluate_purchase: 'Evaluating purchase',
    suggest_price: 'Saving price suggestion',
  };
  return map[name] ?? `Running ${name.replace(/_/g, ' ')}`;
}
