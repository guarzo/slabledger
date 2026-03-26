import { useCallback, useMemo, useState } from 'react';
import Markdown from 'react-markdown';
import { useAdvisorStream } from '../../hooks/useAdvisorStream';
import { useAdvisorCache } from '../../hooks/useAdvisorCache';
import { Button } from '../../ui';
import type { AdvisorAnalysisType } from '../../../types/apiStatus';

interface AIAnalysisWidgetProps {
  endpoint: string;
  body?: Record<string, unknown>;
  title: string;
  buttonLabel: string;
  description?: string;
  cacheType?: AdvisorAnalysisType; // when set, use cached mode instead of streaming
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
  // tables
  "[&_table]:w-full [&_table]:text-xs [&_table]:my-3 [&_th]:text-left [&_th]:text-[var(--text-muted)] [&_th]:pb-1 [&_th]:border-b [&_th]:border-[var(--surface-2)] [&_td]:py-1 [&_td]:pr-3",
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

export default function AIAnalysisWidget({
  endpoint,
  body,
  title,
  buttonLabel,
  description,
  cacheType,
}: AIAnalysisWidgetProps) {
  if (cacheType) {
    return (
      <CachedAnalysisWidget
        cacheType={cacheType}
        title={title}
        buttonLabel={buttonLabel}
        description={description}
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
    />
  );
}

// --- Cached mode ---

function CachedAnalysisWidget({
  cacheType,
  title,
  buttonLabel,
  description,
}: {
  cacheType: AdvisorAnalysisType;
  title: string;
  buttonLabel: string;
  description?: string;
}) {
  const { data, isLoading, refresh } = useAdvisorCache(cacheType);
  const [refreshError, setRefreshError] = useState<string | null>(null);

  const status = data?.status ?? 'empty';
  const isRunning = status === 'running';
  const hasContent = status === 'complete' && !!data?.content;
  const hasError = status === 'error';
  const ageLabel = formatAge(data?.updatedAt);

  const content = data?.content ?? '';
  const renderedMarkdown = useMemo(
    () => hasContent ? <Markdown>{content}</Markdown> : null,
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
      <div className="flex items-center justify-between mb-3">
        <div className="flex items-center gap-2">
          <h2 className="text-sm font-semibold text-[var(--text-muted)] uppercase tracking-wider flex items-center gap-2">
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
}: {
  endpoint: string;
  body?: Record<string, unknown>;
  title: string;
  buttonLabel: string;
  description?: string;
}) {
  const { content, isStreaming, error, toolStatus, run, reset } = useAdvisorStream();

  const handleGenerate = useCallback(() => {
    run(endpoint, body);
  }, [run, endpoint, body]);

  const hasContent = content.length > 0;
  const renderedMarkdown = useMemo(() => <Markdown>{content}</Markdown>, [content]);

  return (
    <div className="p-4 bg-[var(--surface-1)] rounded-xl border border-[var(--surface-2)]">
      <div className="flex items-center justify-between mb-3">
        <h2 className="text-sm font-semibold text-[var(--text-muted)] uppercase tracking-wider flex items-center gap-2">
          <span className="text-base">&#x2728;</span>
          {title}
        </h2>
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
        <div className={AI_PROSE_CLASSES}>
          {renderedMarkdown}
          {isStreaming && (
            <span className="inline-block w-1.5 h-4 bg-[var(--brand-500)] animate-pulse ml-0.5 align-text-bottom" />
          )}
        </div>
      ) : !isStreaming && !error ? (
        <p className="text-xs text-[var(--text-muted)]">
          {description ?? 'Click the button to generate an AI-powered analysis.'}
        </p>
      ) : null}
    </div>
  );
}
