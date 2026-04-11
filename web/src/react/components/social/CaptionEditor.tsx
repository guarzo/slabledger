import { useState, useCallback, useEffect, useRef } from 'react';
import { useUpdateSocialCaption } from '../../queries/useSocialQueries';
import { useAdvisorStream } from '../../hooks/useAdvisorStream';
import { useToast } from '../../contexts/ToastContext';
import { CardShell } from '../../ui/CardShell';
import Button from '../../ui/Button';

const INSTAGRAM_CAPTION_LIMIT = 2200;

interface CaptionEditorProps {
  postId: string;
  caption: string;
  hashtags: string;
  onTitleChange?: (title: string) => void;
}

export default function CaptionEditor({ postId, caption: initialCaption, hashtags: initialHashtags, onTitleChange }: CaptionEditorProps) {
  const [caption, setCaption] = useState(initialCaption);
  const [hashtags, setHashtags] = useState(initialHashtags);
  const [isDirty, setIsDirty] = useState(false);
  const toast = useToast();

  // Sync local state from props when they change (e.g. after server-side save)
  useEffect(() => {
    if (!isDirty) {
      setCaption(initialCaption);
      setHashtags(initialHashtags);
    }
  }, [initialCaption, initialHashtags, isDirty]);

  const updateMutation = useUpdateSocialCaption();
  const { content: streamContent, isStreaming, error: streamError, run: startStream } = useAdvisorStream();
  const wasStreamingRef = useRef(false);

  // When streaming ends successfully, parse the JSON response to get caption, hashtags, and title
  useEffect(() => {
    if (wasStreamingRef.current && !isStreaming && streamContent && !streamError) {
      try {
        const result = JSON.parse(streamContent) as { caption?: string; hashtags?: string; title?: string };
        if (result.caption) setCaption(result.caption);
        if (result.hashtags !== undefined) setHashtags(result.hashtags);
        if (result.title && onTitleChange) onTitleChange(result.title);
      } catch {
        // Fallback: treat as plain text caption (backward compat)
        setCaption(streamContent);
      }
      setIsDirty(false);
    }
    wasStreamingRef.current = isStreaming;
  }, [isStreaming, streamContent, streamError, onTitleChange]);

  const handleCaptionChange = useCallback((value: string) => {
    setCaption(value);
    setIsDirty(true);
  }, []);

  const handleHashtagsChange = useCallback((value: string) => {
    setHashtags(value);
    setIsDirty(true);
  }, []);

  const handleSave = async () => {
    try {
      await updateMutation.mutateAsync({ id: postId, caption, hashtags });
      setIsDirty(false);
      toast.success('Caption saved');
    } catch {
      toast.error('Failed to save caption');
    }
  };

  const handleRegenerate = async () => {
    try {
      await startStream(`/api/social/posts/${postId}/regenerate-caption`);
      // Caption state is updated via the wasStreamingRef useEffect above
    } catch {
      toast.error('Failed to regenerate caption');
    }
  };

  // Show current caption — don't show raw JSON stream content during regeneration
  const displayCaption = caption;
  const totalLength = displayCaption.length + (hashtags ? hashtags.length + 2 : 0);

  const handleCopy = async () => {
    const fullText = hashtags ? `${displayCaption}\n\n${hashtags}` : displayCaption;
    try {
      await navigator.clipboard.writeText(fullText);
      toast.success('Caption copied to clipboard');
    } catch {
      toast.error('Failed to copy');
    }
  };

  return (
    <CardShell padding="md">
      <div className="space-y-4">
        <div className="flex items-center justify-between">
          <h3 className="text-sm font-medium text-[var(--text)]">Caption</h3>
          <div className="flex gap-2">
            <Button size="sm" variant="ghost" onClick={handleRegenerate} loading={isStreaming}>
              Regenerate
            </Button>
            <Button size="sm" variant="ghost" onClick={handleCopy}>
              Copy
            </Button>
          </div>
        </div>

        <textarea
          value={displayCaption}
          onChange={(e) => handleCaptionChange(e.target.value)}
          disabled={isStreaming}
          rows={8}
          className="w-full bg-[var(--surface-0)] border border-[var(--surface-2)] rounded-lg px-3 py-2 text-sm text-[var(--text)] placeholder:text-[var(--text-muted)] focus:outline-none focus:ring-1 focus:ring-[var(--brand-500)] resize-none"
          placeholder="Caption text..."
        />

        <div>
          <label htmlFor={`${postId}-hashtags-input`} className="text-xs text-[var(--text-muted)] block mb-1">Hashtags</label>
          <input
            id={`${postId}-hashtags-input`}
            type="text"
            value={hashtags}
            onChange={(e) => handleHashtagsChange(e.target.value)}
            disabled={isStreaming}
            className="w-full bg-[var(--surface-0)] border border-[var(--surface-2)] rounded-lg px-3 py-2 text-sm text-[var(--text)] placeholder:text-[var(--text-muted)] focus:outline-none focus:ring-1 focus:ring-[var(--brand-500)]"
            placeholder="#PSAgraded #PokemonCards #CardYeti"
          />
        </div>

        <div className="flex items-center justify-between">
          <span className={`text-xs ${totalLength > INSTAGRAM_CAPTION_LIMIT ? 'text-red-400' : 'text-[var(--text-muted)]'}`}>
            {totalLength} / {INSTAGRAM_CAPTION_LIMIT}
          </span>
          {isDirty && (
            <Button size="sm" variant="primary" onClick={handleSave} loading={updateMutation.isPending}>
              Save
            </Button>
          )}
        </div>
      </div>
    </CardShell>
  );
}
