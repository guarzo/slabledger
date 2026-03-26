interface AIBackgroundProps {
  url?: string;
  dimming?: number;
}

export default function AIBackground({ url, dimming = 0.3 }: AIBackgroundProps) {
  if (!url) return null;
  return (
    <>
      <img
        src={url}
        alt=""
        aria-hidden
        className="absolute inset-0 w-full h-full object-cover z-0"
        crossOrigin="anonymous"
      />
      <div
        className="absolute inset-0 z-[1]"
        style={{ background: `rgba(0,0,0,${dimming})` }}
      />
    </>
  );
}
