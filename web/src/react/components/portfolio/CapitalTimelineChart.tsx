import { useState, useMemo } from 'react';
import { LineChart, Line, ReferenceLine, XAxis, YAxis, Tooltip, ResponsiveContainer } from 'recharts';
import type { CapitalTimeline } from '../../../types/campaigns';
import CollapsibleHeader from './CollapsibleHeader';

export default function CapitalTimelineChart({ data }: { data: CapitalTimeline }) {
  const [open, setOpen] = useState(true);

  const chartData = useMemo(() => {
    return (data.dataPoints ?? []).map(dp => ({
      date: dp.date,
      spend: dp.cumulativeSpendCents / 100,
      recovery: dp.cumulativeRecoveryCents / 100,
      outstanding: dp.outstandingCents / 100,
    }));
  }, [data.dataPoints]);

  if (chartData.length === 0) return null;

  return (
    <div className="mb-6 p-4 bg-[var(--surface-1)] rounded-xl border border-[var(--surface-2)]">
      <CollapsibleHeader title="Capital Timeline" open={open} onToggle={() => setOpen(!open)} />
      {open && (
        <div className="mt-3">
          <ResponsiveContainer width="100%" height={260}>
            <LineChart data={chartData} margin={{ top: 8, right: 8, bottom: 4, left: 0 }}>
              <XAxis
                dataKey="date"
                tick={{ fontSize: 10, fill: 'var(--text-muted)' }}
                axisLine={false}
                tickLine={false}
                tickFormatter={(v: string) => {
                  const d = new Date(v + 'T12:00:00');
                  return `${d.getMonth() + 1}/${d.getDate()}`;
                }}
                interval="preserveStartEnd"
              />
              <YAxis
                tick={{ fontSize: 10, fill: 'var(--text-muted)' }}
                axisLine={false}
                tickLine={false}
                tickFormatter={(v: number) => `$${v >= 1000 ? `${(v / 1000).toFixed(1)}k` : v.toFixed(0)}`}
                width={52}
              />
              <Tooltip
                contentStyle={{
                  background: 'var(--surface-2)',
                  border: '1px solid var(--surface-3, #444)',
                  borderRadius: 8,
                  fontSize: 12,
                }}
                labelStyle={{ color: 'var(--text)', fontWeight: 600 }}
                formatter={(value, name) => {
                  const v = Number(value);
                  const label = name === 'spend' ? 'Cumulative Spend'
                    : name === 'recovery' ? 'Cumulative Recovery'
                    : 'Outstanding';
                  return [`$${v.toFixed(2)}`, label];
                }}
                labelFormatter={(label) => {
                  const d = new Date(String(label) + 'T12:00:00');
                  return d.toLocaleDateString('en-US', { month: 'short', day: 'numeric', year: 'numeric' });
                }}
              />
              {(data.invoiceDates ?? []).map(invDate => (
                <ReferenceLine
                  key={invDate}
                  x={invDate}
                  stroke="var(--text-muted)"
                  strokeDasharray="3 3"
                  strokeOpacity={0.5}
                />
              ))}
              <Line type="monotone" dataKey="spend" stroke="var(--chart-line-spend)" strokeWidth={2} dot={false} name="spend" />
              <Line type="monotone" dataKey="recovery" stroke="var(--chart-line-recovery)" strokeWidth={2} dot={false} name="recovery" />
              <Line type="monotone" dataKey="outstanding" stroke="var(--chart-line-outstanding)" strokeWidth={2} dot={false} name="outstanding" />
            </LineChart>
          </ResponsiveContainer>
          <div className="flex flex-wrap justify-center gap-4 mt-2 text-xs text-[var(--text-muted)]">
            <span className="flex items-center gap-1">
              <span className="inline-block w-3 h-0.5 bg-[var(--chart-line-spend)] rounded" /> Spend
            </span>
            <span className="flex items-center gap-1">
              <span className="inline-block w-3 h-0.5 bg-[var(--chart-line-recovery)] rounded" /> Recovery
            </span>
            <span className="flex items-center gap-1">
              <span className="inline-block w-3 h-0.5 bg-[var(--chart-line-outstanding)] rounded" /> Outstanding
            </span>
            {Array.isArray(data.invoiceDates) && data.invoiceDates.length > 0 && (
              <span className="flex items-center gap-1">
                <span className="inline-block w-3 h-0 border-t border-dashed border-[var(--text-muted)]" /> Invoice
              </span>
            )}
          </div>
        </div>
      )}
    </div>
  );
}
