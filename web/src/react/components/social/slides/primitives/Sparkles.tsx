const STAR_CLIP = 'polygon(50% 0%, 61% 35%, 98% 35%, 68% 57%, 79% 91%, 50% 70%, 21% 91%, 32% 57%, 2% 35%, 39% 35%)';

interface StarProps {
  top: string;
  left?: string;
  right?: string;
  size: number;
  color: string;
  opacity: number;
}

function Star({ top, left, right, size, color, opacity }: StarProps) {
  return (
    <div
      className="absolute"
      style={{
        top, left, right,
        width: size, height: size,
        clipPath: STAR_CLIP,
        background: color,
        opacity,
        filter: `drop-shadow(0 0 ${Math.ceil(size / 2)}px ${color})`,
      }}
    />
  );
}

export default function Sparkles() {
  return (
    <div className="absolute inset-0 pointer-events-none z-[6]">
      <Star top="15%" left="12%" size={8} color="white" opacity={0.6} />
      <Star top="25%" right="15%" size={12} color="#a5b4fc" opacity={0.7} />
      <Star top="60%" left="8%" size={6} color="#10b981" opacity={0.5} />
      <Star top="40%" right="8%" size={5} color="white" opacity={0.4} />
      <Star top="70%" right="20%" size={7} color="#a5b4fc" opacity={0.5} />
      <Star top="50%" left="85%" size={4} color="#10b981" opacity={0.3} />
      <Star top="80%" left="30%" size={6} color="white" opacity={0.35} />
    </div>
  );
}
