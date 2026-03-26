export default function TrendLines({ intensity = 'full' }: { intensity?: 'full' | 'subtle' }) {
  const opacity = intensity === 'full' ? 0.15 : 0.08;
  return (
    <div className="absolute inset-0 pointer-events-none" style={{ opacity }}>
      <svg className="w-full h-full" viewBox="0 0 400 400" preserveAspectRatio="none">
        {/* Upward trend lines */}
        <polyline
          points="0,350 80,320 160,280 240,200 320,150 400,80"
          fill="none" stroke="#10b981" strokeWidth="2" strokeDasharray="8,4"
        />
        <polyline
          points="0,380 100,350 200,300 280,220 360,160 400,120"
          fill="none" stroke="#10b981" strokeWidth="1" opacity="0.5"
        />
        {/* Grid lines */}
        <line x1="0" y1="100" x2="400" y2="100" stroke="rgba(255,255,255,0.3)" strokeWidth="1" />
        <line x1="0" y1="200" x2="400" y2="200" stroke="rgba(255,255,255,0.3)" strokeWidth="1" />
        <line x1="0" y1="300" x2="400" y2="300" stroke="rgba(255,255,255,0.3)" strokeWidth="1" />
      </svg>
      {/* Decorative trend arrow */}
      <div className="absolute top-[18%] right-[10%]" style={{ opacity: 0.8 }}>
        <svg width="80" height="80" viewBox="0 0 80 80">
          <path d="M20 60 L40 15 L60 60 Z" fill="none" stroke="#10b981" strokeWidth="3" />
          <path d="M30 50 L40 25 L50 50 Z" fill="#10b981" opacity="0.3" />
        </svg>
      </div>
    </div>
  );
}
