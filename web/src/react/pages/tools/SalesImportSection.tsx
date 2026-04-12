import { SectionErrorBoundary } from '../../ui';
import ImportSalesTab from './ImportSalesTab';

export default function SalesImportSection() {
  return (
    <div id="sales-panel" role="region" aria-label="Import Sales">
      <SectionErrorBoundary sectionName="Import Sales">
        <ImportSalesTab />
      </SectionErrorBoundary>
    </div>
  );
}
