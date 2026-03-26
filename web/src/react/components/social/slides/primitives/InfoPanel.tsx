interface InfoPanelProps {
  title: string;
  subtitle: string;
}

export default function InfoPanel({ title, subtitle }: InfoPanelProps) {
  return (
    <div className="absolute bottom-0 left-0 right-0 z-[8]"
      style={{ background: 'linear-gradient(transparent, rgba(0,0,0,0.9))' }}>
      <div className="px-4 pb-3.5 pt-10">
        <div className="text-[15px] font-bold text-white mb-0.5">{title}</div>
        <div className="flex justify-between items-center">
          <span className="text-[10px] text-white/50">{subtitle}</span>
          <span className="text-[9px] text-white/30">cardyeti.com</span>
        </div>
      </div>
    </div>
  );
}
