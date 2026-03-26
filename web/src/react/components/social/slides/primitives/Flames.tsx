export default function Flames({ intensity = 'full' }: { intensity?: 'full' | 'subtle' }) {
  const opacity = intensity === 'full' ? 1 : 0.4;
  return (
    <div className="absolute inset-0 pointer-events-none" style={{ opacity }}>
      {/* Fire glow rising from bottom */}
      <div className="absolute bottom-[-20px] left-[-10%] right-[-10%] h-[45%]">
        <div className="absolute bottom-0 left-[10%] w-[80px] h-[160px] rounded-full"
          style={{ background: 'radial-gradient(ellipse at bottom, rgba(255,100,0,0.5) 0%, rgba(255,60,0,0.3) 30%, transparent 70%)', filter: 'blur(15px)', transform: 'rotate(-5deg)' }} />
        <div className="absolute bottom-0 left-[30%] w-[100px] h-[200px] rounded-full"
          style={{ background: 'radial-gradient(ellipse at bottom, rgba(255,160,0,0.4) 0%, rgba(255,80,0,0.2) 40%, transparent 70%)', filter: 'blur(20px)' }} />
        <div className="absolute bottom-0 left-[55%] w-[90px] h-[180px] rounded-full"
          style={{ background: 'radial-gradient(ellipse at bottom, rgba(255,60,0,0.5) 0%, rgba(200,30,0,0.3) 35%, transparent 70%)', filter: 'blur(18px)', transform: 'rotate(5deg)' }} />
        <div className="absolute bottom-0 right-[10%] w-[70px] h-[140px] rounded-full"
          style={{ background: 'radial-gradient(ellipse at bottom, rgba(255,120,0,0.4) 0%, rgba(255,60,0,0.2) 40%, transparent 70%)', filter: 'blur(12px)', transform: 'rotate(8deg)' }} />
      </div>
      {/* Ember dots */}
      <div className="absolute bottom-[30%] left-[25%] w-1 h-1 rounded-full bg-orange-500 shadow-[0_0_6px_#ff6600] opacity-70" />
      <div className="absolute bottom-[40%] left-[60%] w-[3px] h-[3px] rounded-full bg-amber-400 shadow-[0_0_5px_#ff8800] opacity-50" />
      <div className="absolute bottom-[35%] right-[25%] w-[5px] h-[5px] rounded-full bg-orange-600 shadow-[0_0_8px_#ff4400] opacity-60" />
      <div className="absolute bottom-[50%] left-[45%] w-1 h-1 rounded-full bg-yellow-500 shadow-[0_0_4px_#ffaa00] opacity-40" />
    </div>
  );
}
