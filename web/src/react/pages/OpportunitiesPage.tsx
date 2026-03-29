import PicksList from '../components/picks/PicksList';
import AcquisitionWatchlist from '../components/picks/AcquisitionWatchlist';

export default function OpportunitiesPage() {
  return (
    <div className="max-w-4xl mx-auto px-4 space-y-8">
      <div>
        <h1 className="text-2xl font-bold text-gradient text-gradient-premium mb-2">Opportunities</h1>
        <p className="text-sm text-[var(--text-muted)]">AI-generated acquisition recommendations updated daily.</p>
      </div>
      <section>
        <h2 className="text-lg font-semibold text-[var(--text)] mb-4">Today's Picks</h2>
        <PicksList />
      </section>
      <section>
        <AcquisitionWatchlist />
      </section>
    </div>
  );
}
