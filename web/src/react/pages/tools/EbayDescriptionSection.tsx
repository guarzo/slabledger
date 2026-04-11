import { SectionErrorBoundary } from '../../ui';
import EbayExportTab from './EbayExportTab';

export default function EbayDescriptionSection() {
  return (
    <div id="ebay-panel" role="region" aria-label="eBay Export">
      <SectionErrorBoundary sectionName="eBay Export">
        <EbayExportTab />
      </SectionErrorBoundary>
    </div>
  );
}
