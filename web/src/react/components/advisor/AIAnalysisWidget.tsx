import { useCallback, useEffect, useMemo, useState } from 'react';
import Markdown from 'react-markdown';
import remarkGfm from 'remark-gfm';
import { useAdvisorStream } from '../../hooks/useAdvisorStream';
import { ScoreCardHeader } from './ScoreCardHeader';
import { useAdvisorCache } from '../../hooks/useAdvisorCache';
import { Button } from '../../ui';
import type { AdvisorAnalysisType } from '../../../types/apiStatus';

const remarkPlugins = [remarkGfm];

const markdownComponents = {
  table: ({ children, ...props }: React.ComponentPropsWithoutRef<'table'>) => (
    <div className="table-wrap">
      <table {...props}>{children}</table>
    </div>
  ),
};

interface AIAnalysisWidgetProps {
  endpoint: string;
  body?: Record<string, unknown>;
  title: string;
  buttonLabel: string;
  description?: string;
  cacheType?: AdvisorAnalysisType; // when set, use cached mode instead of streaming
  collapsible?: boolean; // when true, content can be collapsed to save space
}

const AI_PROSE_CLASSES = [
  "ai-prose prose prose-sm prose-invert max-w-none text-[var(--text)] text-sm leading-relaxed",
  // h1 — report title
  "[&_h1]:text-lg [&_h1]:font-bold [&_h1]:text-[var(--text)] [&_h1]:mt-6 [&_h1]:mb-4 [&_h1]:pb-2 [&_h1]:border-b [&_h1]:border-[var(--surface-2)]",
  // h2 — major sections (Performance, Credit, Top Actions, etc.)
  "[&_h2]:text-[15px] [&_h2]:font-bold [&_h2]:text-[var(--brand-400)] [&_h2]:mt-6 [&_h2]:mb-3 [&_h2]:pt-3 [&_h2]:border-t [&_h2]:border-[var(--surface-2)]",
  // h3 — subsections
  "[&_h3]:text-sm [&_h3]:font-semibold [&_h3]:text-[var(--text)] [&_h3]:mt-4 [&_h3]:mb-2",
  // h4 — minor labels
  "[&_h4]:text-xs [&_h4]:font-semibold [&_h4]:text-[var(--text-muted)] [&_h4]:uppercase [&_h4]:tracking-wide [&_h4]:mt-3 [&_h4]:mb-1",
  // inline
  "[&_strong]:text-[var(--text)]",
  // lists
  "[&_ul]:space-y-1.5 [&_ul]:my-2 [&_ol]:space-y-1.5 [&_ol]:my-2 [&_li]:text-[var(--text)] [&_li]:leading-relaxed",
  // paragraphs
  "[&_p]:mb-3 [&_p]:leading-relaxed",
  // tables — wrapped in scrollable container via component override
  "[&_.table-wrap]:overflow-x-auto [&_.table-wrap]:my-3 [&_.table-wrap]:rounded-lg [&_.table-wrap]:border [&_.table-wrap]:border-[var(--surface-2)]",
  "[&_table]:w-full [&_table]:text-xs [&_table]:border-collapse",
  "[&_th]:text-left [&_th]:text-[var(--text-muted)] [&_th]:font-semibold [&_th]:px-3 [&_th]:py-2 [&_th]:border-b [&_th]:border-[var(--surface-2)] [&_th]:bg-[var(--surface-2)]/50 [&_th]:whitespace-nowrap",
  "[&_td]:px-3 [&_td]:py-2 [&_td]:border-b [&_td]:border-[var(--surface-2)]/50 [&_td]:align-top",
  "[&_tr:last-child_td]:border-b-0",
  "[&_tr:hover_td]:bg-[var(--surface-2)]/30",
  // code + hr
  "[&_code]:text-[var(--brand-400)] [&_code]:text-xs [&_hr]:border-[var(--surface-2)] [&_hr]:my-5",
].join(" ");

function formatAge(updatedAt?: string): string {
  if (!updatedAt) return '';
  const ms = Date.now() - new Date(updatedAt).getTime();
  const mins = Math.floor(ms / 60_000);
  if (mins < 1) return 'Updated just now';
  if (mins < 60) return `Updated ${mins}m ago`;
  const hours = Math.floor(mins / 60);
  if (hours < 24) return `Updated ${hours}h ago`;
  return `Updated ${Math.floor(hours / 24)}d ago`;
}

function useCollapsed(key: string, defaultCollapsed: boolean) {
  const storageKey = `ai-widget-collapsed-${key}`;
  const [collapsed, setCollapsed] = useState(() => {
    try {
      const stored = localStorage.getItem(storageKey);
      return stored !== null ? stored === 'true' : defaultCollapsed;
    } catch {
      return defaultCollapsed;
    }
  });
  useEffect(() => {
    try { localStorage.setItem(storageKey, String(collapsed)); } catch { /* noop */ }
  }, [storageKey, collapsed]);
  return [collapsed, setCollapsed] as const;
}

function CollapseChevron({ collapsed }: { collapsed: boolean }) {
  return (
    <svg
      xmlns="http://www.w3.org/2000/svg"
      viewBox="0 0 24 24"
      fill="none"
      stroke="currentColor"
      strokeWidth="2"
      strokeLinecap="round"
      strokeLinejoin="round"
      className={`w-4 h-4 text-[var(--text-muted)] transition-transform duration-200 ${collapsed ? '' : 'rotate-180'}`}
      aria-hidden="true"
    >
      <polyline points="6 9 12 15 18 9" />
    </svg>
  );
}

export default function AIAnalysisWidget({
  endpoint,
  body,
  title,
  buttonLabel,
  description,
  cacheType,
  collapsible,
}: AIAnalysisWidgetProps) {
  if (cacheType) {
    return (
      <CachedAnalysisWidget
        cacheType={cacheType}
        title={title}
        buttonLabel={buttonLabel}
        description={description}
        collapsible={collapsible}
      />
    );
  }

  return (
    <StreamingAnalysisWidget
      endpoint={endpoint}
      body={body}
      title={title}
      buttonLabel={buttonLabel}
      description={description}
      collapsible={collapsible}
    />
  );
}

// --- Cached mode ---

function CachedAnalysisWidget({
  cacheType,
  title,
  buttonLabel,
  description,
  collapsible,
}: {
  cacheType: AdvisorAnalysisType;
  title: string;
  buttonLabel: string;
  description?: string;
  collapsible?: boolean;
}) {
  const { data, isLoading, refresh } = useAdvisorCache(cacheType);
  const [refreshError, setRefreshError] = useState<string | null>(null);
  const [collapsed, setCollapsed] = useCollapsed(cacheType, true);

  const status = data?.status ?? 'empty';
  const isRunning = status === 'running';
  const hasContent = status === 'complete' && !!data?.content;
  const hasError = status === 'error';
  const ageLabel = formatAge(data?.updatedAt);
  const showCollapse = collapsible && hasContent;

  const content = data?.content ?? '';
  const renderedMarkdown = useMemo(
    () => hasContent ? <Markdown remarkPlugins={remarkPlugins} components={markdownComponents}>{content}</Markdown> : null,
    [hasContent, content],
  );

  const handleRefresh = useCallback(async () => {
    setRefreshError(null);
    try {
      await refresh();
    } catch {
      setRefreshError('Failed to trigger refresh. Please try again.');
    }
  }, [refresh]);

  return (
    <div className="p-4 bg-[var(--surface-1)] rounded-xl border border-[var(--surface-2)]">
      <div className={`flex items-center justify-between ${showCollapse && collapsed ? '' : 'mb-3'}`}>
        <div className="flex items-center gap-2">
          {showCollapse && (
            <button
              type="button"
              onClick={() => setCollapsed(c => !c)}
              className="p-0.5 -ml-1 hover:text-[var(--brand-500)] transition-colors"
              aria-expanded={!collapsed}
              aria-label={collapsed ? 'Expand' : 'Collapse'}
            >
              <CollapseChevron collapsed={collapsed} />
            </button>
          )}
          <h2
            className={`text-sm font-semibold text-[var(--text-muted)] uppercase tracking-wider flex items-center gap-2 ${showCollapse ? 'cursor-pointer hover:text-[var(--brand-500)] transition-colors' : ''}`}
            onClick={showCollapse ? () => setCollapsed(c => !c) : undefined}
          >
            <span className="text-base">&#x2728;</span>
            {title}
          </h2>
          {ageLabel && hasContent && (
            <span className="text-[10px] text-[var(--text-muted)] bg-[var(--surface-2)] px-1.5 py-0.5 rounded">
              {ageLabel}
            </span>
          )}
        </div>
        <Button
          variant="primary"
          size="sm"
          onClick={() => { void handleRefresh(); }}
          disabled={isRunning}
          loading={isRunning || isLoading}
        >
          {hasContent ? 'Refresh' : buttonLabel}
        </Button>
      </div>

      {!(showCollapse && collapsed) && (
        <>
          {isRunning && (
            <div className="flex items-center gap-2 mb-3 px-3 py-2 bg-[var(--surface-2)]/50 rounded-lg text-xs text-[var(--text-muted)]">
              <span className="inline-block w-2 h-2 bg-[var(--brand-500)] rounded-full animate-pulse" />
              Generating analysis... this may take a minute
            </div>
          )}

          {(hasError || refreshError) && (
            <div className="mb-3 px-3 py-2 bg-[var(--danger)]/10 border border-[var(--danger)]/20 rounded-lg text-xs text-[var(--danger)]">
              {refreshError ?? `Analysis failed: ${data?.errorMessage ?? 'Unknown error'}`}
            </div>
          )}

          {hasContent ? (
            <div className={AI_PROSE_CLASSES}>
              {renderedMarkdown}
            </div>
          ) : !isRunning && !hasError && !refreshError ? (
            <p className="text-xs text-[var(--text-muted)]">
              {description ?? 'Click the button to generate an AI-powered analysis.'}
            </p>
          ) : null}
        </>
      )}
    </div>
  );
}

// --- Streaming mode (original) ---

function StreamingAnalysisWidget({
  endpoint,
  body,
  title,
  buttonLabel,
  description,
  collapsible,
}: {
  endpoint: string;
  body?: Record<string, unknown>;
  title: string;
  buttonLabel: string;
  description?: string;
  collapsible?: boolean;
}) {
  const { content, isStreaming, error, toolStatus, scoreCard, run, reset } = useAdvisorStream();
  const [collapsed, setCollapsed] = useCollapsed(endpoint, true);

  const handleGenerate = useCallback(() => {
    run(endpoint, body);
  }, [run, endpoint, body]);

  const hasContent = content.length > 0;
  const showCollapse = collapsible && hasContent && !isStreaming;
  const renderedMarkdown = useMemo(() => <Markdown remarkPlugins={remarkPlugins} components={markdownComponents}>{content}</Markdown>, [content]);

  return (
    <div className="p-4 bg-[var(--surface-1)] rounded-xl border border-[var(--surface-2)]">
      <div className={`flex items-center justify-between ${showCollapse && collapsed ? '' : 'mb-3'}`}>
        <div className="flex items-center gap-2">
          {showCollapse && (
            <button
              type="button"
              onClick={() => setCollapsed(c => !c)}
              className="p-0.5 -ml-1 hover:text-[var(--brand-500)] transition-colors"
              aria-expanded={!collapsed}
              aria-label={collapsed ? 'Expand' : 'Collapse'}
            >
              <CollapseChevron collapsed={collapsed} />
            </button>
          )}
          <h2
            className={`text-sm font-semibold text-[var(--text-muted)] uppercase tracking-wider flex items-center gap-2 ${showCollapse ? 'cursor-pointer hover:text-[var(--brand-500)] transition-colors' : ''}`}
            onClick={showCollapse ? () => setCollapsed(c => !c) : undefined}
          >
            <span className="text-base">&#x2728;</span>
            {title}
          </h2>
        </div>
        <div className="flex items-center gap-2">
          {hasContent && !isStreaming && (
            <Button variant="ghost" size="sm" onClick={reset}>Clear</Button>
          )}
          <Button
            variant="primary"
            size="sm"
            onClick={handleGenerate}
            disabled={isStreaming}
            loading={isStreaming}
          >
            {hasContent ? 'Regenerate' : buttonLabel}
          </Button>
        </div>
      </div>

      {!(showCollapse && collapsed) && (
        <>
          {toolStatus && (
            <div className="flex items-center gap-2 mb-3 px-3 py-2 bg-[var(--surface-2)]/50 rounded-lg text-xs text-[var(--text-muted)]">
              <span className="inline-block w-2 h-2 bg-[var(--brand-500)] rounded-full animate-pulse" />
              {toolStatus}...
            </div>
          )}

          {error && (
            <div className="mb-3 px-3 py-2 bg-[var(--danger)]/10 border border-[var(--danger)]/20 rounded-lg text-xs text-[var(--danger)]">
              {error}
            </div>
          )}

          {hasContent ? (
            <>
              {scoreCard && <ScoreCardHeader scoreCard={scoreCard} />}
              <div className={AI_PROSE_CLASSES}>
                {renderedMarkdown}
                {isStreaming && (
                  <span className="inline-block w-1.5 h-4 bg-[var(--brand-500)] animate-pulse ml-0.5 align-text-bottom" />
                )}
              </div>
            </>
          ) : !isStreaming && !error ? (
            <p className="text-xs text-[var(--text-muted)]">
              {description ?? 'Click the button to generate an AI-powered analysis.'}
            </p>
          ) : null}
        </>
      )}
    </div>
  );
}
