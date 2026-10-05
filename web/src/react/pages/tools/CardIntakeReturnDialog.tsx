import { ConfirmDialog } from '../../ui';

export default function CardIntakeReturnDialog({ certNumber, loading, onConfirm, onCancel }: {
  certNumber?: string;
  loading: boolean;
  onConfirm: () => void;
  onCancel: () => void;
}) {
  return (
    <ConfirmDialog
      open={!!certNumber}
      title="Confirm physical return?"
      message={`Confirm that slab ${certNumber ?? ''} is physically back and its refund or return is resolved. This reverses the recorded sale; it does not list the slab.`}
      confirmLabel="Confirm return"
      cancelLabel="Cancel"
      loading={loading}
      onConfirm={onConfirm}
      onCancel={onCancel}
    />
  );
}
