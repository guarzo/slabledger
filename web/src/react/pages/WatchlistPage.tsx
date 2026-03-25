import WatchlistSection from '../components/watchlist/WatchlistSection';

export default function WatchlistPage() {
  return (
    <div className="max-w-6xl mx-auto px-4">
      <h1 className="text-2xl font-bold text-gradient text-gradient-premium mb-6">Watchlist</h1>
      <WatchlistSection />
    </div>
  );
}
