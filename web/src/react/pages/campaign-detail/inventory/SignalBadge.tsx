import { signalLabel, signalBgColor } from '../../../utils/formatters';

interface SignalBadgeProps {
  direction: string;
  deltaPct?: number | null;
}

export default function SignalBadge({ direction }: SignalBadgeProps) {
  return (
    <span className={`text-xs font-medium px-2 py-0.5 rounded ${signalBgColor(direction)}`}>
      {signalLabel(direction)}
    </span>
  );
}
