export type LoaderSize = "sm" | "md" | "lg";

export interface PokeballLoaderProps {
  size?: LoaderSize;
  text?: string | null;
}

/**
 * Pokéball Loading Animation
 * A subtle, themed loading indicator for the SlabLedger
 */
export default function PokeballLoader({ size = "md", text = "Loading..." }: PokeballLoaderProps) {
  const sizeClasses: Record<LoaderSize, string> = {
    sm: "w-8 h-8",
    md: "w-12 h-12",
    lg: "w-16 h-16",
  };

  const containerClasses = size === "sm" ? "gap-2" : "gap-4";

  return (
    <div data-testid="pokeball-loader" className={`flex flex-col items-center justify-center ${containerClasses}`}>
      <div className={`pokeball-loader ${sizeClasses[size]}`}>
        <div className="pokeball-loader__ball">
          <div className="pokeball-loader__top"></div>
          <div className="pokeball-loader__middle">
            <div className="pokeball-loader__button"></div>
          </div>
          <div className="pokeball-loader__bottom"></div>
        </div>
      </div>
      {text && (
        <p className="text-sm text-[var(--text-muted)] animate-pulse">
          {text}
        </p>
      )}
    </div>
  );
}
