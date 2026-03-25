import type { KeyboardEvent } from "react";
import { useState, useEffect, useRef, useCallback } from "react";
import { Popover } from 'radix-ui';
import { useDebounce } from "./hooks/useDebounce";
import { useUserPreferences } from "./contexts/UserPreferencesContext";
import type { CardSearchResult, SearchCardsResponse } from "../js/api";

export interface PriceCheckFormProps {
  onCardSelect?: (card: CardSearchResult) => void;
  onSearch: (query: string) => Promise<SearchCardsResponse>;
}

/**
 * PriceCheckForm Component
 * Smart card search with autocomplete and price check functionality
 * Now integrated with UserPreferencesContext for persistent recent searches
 */
export default function PriceCheckForm({ onCardSelect, onSearch }: PriceCheckFormProps) {
  const { preferences, addRecentPriceCheck, clearRecentPriceChecks } = useUserPreferences();
  const { recentPriceChecks } = preferences;

  const [searchQuery, setSearchQuery] = useState("");
  const [results, setResults] = useState<CardSearchResult[]>([]);
  const [selectedIndex, setSelectedIndex] = useState(-1);
  const [isSearching, setIsSearching] = useState(false);
  const [error, setError] = useState<string | null>(null);
  const [showResults, setShowResults] = useState(false);

  const inputRef = useRef<HTMLInputElement>(null);
  const debouncedQuery = useDebounce(searchQuery, 300);

  // Perform search when debounced query changes
  useEffect(() => {
    if (debouncedQuery.trim().length < 2) {
      setResults([]);
      setShowResults(false);
      return;
    }

    const performSearch = async () => {
      setIsSearching(true);
      setError(null);
      try {
        const response = await onSearch(debouncedQuery);
        if (response?.cards) {
          setResults(response.cards);
          setShowResults(true);
          setSelectedIndex(-1);
        } else {
          setResults([]);
          setShowResults(false);
        }
      } catch {
        setError("Search failed. Please try again.");
        setResults([]);
        setShowResults(false);
      } finally {
        setIsSearching(false);
      }
    };

    performSearch();
  }, [debouncedQuery, onSearch]);

  // Handle card selection
  const handleCardSelect = useCallback((card: CardSearchResult) => {
    // Blur input to dismiss mobile keyboard and prevent focus-related issues
    inputRef.current?.blur();

    setSearchQuery(card.name);
    setShowResults(false);
    setResults([]);
    setSelectedIndex(-1);

    // Save to recent searches via context (handles max 5 automatically)
    addRecentPriceCheck({
      id: card.id,
      name: card.name,
      setName: card.setName,
      number: card.number,
      imageUrl: card.imageUrl,
    });

    onCardSelect?.(card);
  }, [addRecentPriceCheck, onCardSelect]);

  // Handle keyboard navigation
  const handleKeyDown = (e: KeyboardEvent<HTMLInputElement>) => {
    if (!showResults || results.length === 0) return;

    switch (e.key) {
      case "ArrowDown":
        e.preventDefault();
        setSelectedIndex((prev) =>
          prev < results.length - 1 ? prev + 1 : prev
        );
        break;
      case "ArrowUp":
        e.preventDefault();
        setSelectedIndex((prev) => (prev > 0 ? prev - 1 : -1));
        break;
      case "Enter":
        e.preventDefault();
        if (selectedIndex >= 0 && results[selectedIndex]) {
          handleCardSelect(results[selectedIndex]);
        }
        break;
      case "Escape":
        setShowResults(false);
        setSelectedIndex(-1);
        break;
      default:
        break;
    }
  };

  // Clear search
  const handleClear = () => {
    setSearchQuery("");
    setResults([]);
    setShowResults(false);
    setSelectedIndex(-1);
    setError(null);
    inputRef.current?.focus();
  };

  // Clear recent searches
  const handleClearRecent = () => {
    clearRecentPriceChecks();
  };

  return (
    <div className="space-y-4">
      {/* Search Input */}
      <Popover.Root
        open={showResults && results.length > 0}
        onOpenChange={(open) => { if (!open) { setShowResults(false); setSelectedIndex(-1); } }}
      >
        <Popover.Anchor asChild>
          <div className="relative">
            <div className="absolute inset-y-0 left-0 flex items-center pl-3 pointer-events-none">
              <span className="text-xl" role="presentation" aria-hidden="true">🔍</span>
            </div>
            <input
              ref={inputRef}
              type="text"
              value={searchQuery}
              onChange={(e) => setSearchQuery(e.target.value)}
              onKeyDown={handleKeyDown}
              onFocus={() => setShowResults(results.length > 0)}
              placeholder="Search any card... (e.g., 'Charizard Base Set', 'Pikachu 025')"
              className="w-full pl-12 pr-12 py-3 bg-[var(--surface-2)] border border-[var(--surface-2)] rounded-lg text-sm focus:outline-none focus:ring-2 focus:ring-brand-start focus:border-[var(--brand-500)]"
              role="combobox"
              aria-label="Search for Pokemon cards"
              aria-autocomplete="list"
              aria-controls="searchResults"
              aria-expanded={showResults}
            />
            {searchQuery && (
              <button
                type="button"
                onClick={handleClear}
                className="absolute inset-y-0 right-0 flex items-center pr-3 hover:text-danger transition-colors"
                aria-label="Clear search"
              >
                ✕
              </button>
            )}
            {isSearching && (
              <div className="absolute inset-y-0 right-10 flex items-center pr-3">
                <div className="w-4 h-4 border-2 border-brand-start border-t-transparent rounded-full animate-spin" />
              </div>
            )}
          </div>
        </Popover.Anchor>
        <Popover.Portal>
          <Popover.Content
            id="searchResults"
            role="listbox"
            sideOffset={8}
            align="start"
            onOpenAutoFocus={(e) => e.preventDefault()}
            className="w-[var(--radix-popover-trigger-width)] bg-[var(--surface-1)] border border-[var(--surface-2)] rounded-lg shadow-xl max-h-96 overflow-y-auto z-50 data-[state=open]:animate-[fadeIn_100ms_ease-out]"
          >
            {results.map((card, index) => (
              <button
                key={card.id}
                type="button"
                onClick={() => handleCardSelect(card)}
                onMouseEnter={() => setSelectedIndex(index)}
                className={`w-full px-4 py-3 flex items-center gap-3 hover:bg-[var(--surface-hover)] transition-colors text-left ${
                  index === selectedIndex ? "bg-[var(--surface-2)]" : ""
                }`}
                role="option"
                aria-selected={index === selectedIndex}
              >
                {card.imageUrl && (
                  <img
                    src={card.imageUrl}
                    alt=""
                    className="w-12 h-16 object-cover rounded"
                    loading="lazy"
                  />
                )}
                <div className="flex-1 min-w-0">
                  <div className="font-medium truncate">{card.name}</div>
                  <div className="text-sm text-[var(--text-muted)]">
                    {card.setName} • #{card.number}
                  </div>
                </div>
              </button>
            ))}
          </Popover.Content>
        </Popover.Portal>
      </Popover.Root>

      {/* Error Message */}
      {error && (
        <div className="p-3 bg-danger/10 border border-danger/20 rounded-lg text-sm text-danger">
          {error}
        </div>
      )}

      {/* Recent Searches */}
      {!showResults && recentPriceChecks.length > 0 && (
        <div className="space-y-2">
          <div className="flex items-center justify-between">
            <h4 className="text-sm font-medium text-[var(--text-muted)]">
              Recent Searches
            </h4>
            <button
              type="button"
              onClick={handleClearRecent}
              className="text-xs text-[var(--text-subtle)] hover:text-[var(--text-muted)] transition-colors"
            >
              Clear
            </button>
          </div>
          <div className="flex flex-wrap gap-2">
            {recentPriceChecks.map((card) => (
              <button
                key={card.id}
                type="button"
                onClick={() => handleCardSelect(card)}
                className="px-3 py-1.5 bg-[var(--surface-2)] border border-[var(--surface-2)] rounded-full text-xs hover:bg-[var(--surface-hover)] transition-colors"
              >
                {card.name}
              </button>
            ))}
          </div>
        </div>
      )}

      {/* Help Text */}
      {!searchQuery && (
        <p className="text-sm text-[var(--text-subtle)]">
          Start typing to search for a card by name, set, or number
        </p>
      )}
    </div>
  );
}
