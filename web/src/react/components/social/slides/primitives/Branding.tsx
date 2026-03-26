import logo from '../../../../../assets/card-yeti-business-logo.png';

export function AccentBar({ gradientBar }: { gradientBar: string }) {
  return (
    <div className={`absolute top-0 left-0 right-0 h-[3px] bg-gradient-to-r ${gradientBar} z-10`} />
  );
}

export function Logo({ size = 'sm' }: { size?: 'sm' | 'lg' }) {
  const width = size === 'lg' ? 'w-[200px]' : 'w-[80px]';
  return <img src={logo} alt="Card Yeti" className={width} crossOrigin="anonymous" />;
}

export function LogoText() {
  return (
    <span className="text-[11px] font-bold text-white/70 tracking-[1px]">
      CARD YETI
    </span>
  );
}

export function Footer() {
  return (
    <span className="text-xs text-white/40">cardyeti.com</span>
  );
}

export function SlideCounter({ current, total }: { current: number; total: number }) {
  return (
    <span className="text-xs text-white/30">{current} / {total}</span>
  );
}
