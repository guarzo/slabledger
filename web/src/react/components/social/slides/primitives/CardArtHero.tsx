interface CardArtHeroProps {
  imageUrl: string;
  cardName: string;
  grader: string;
  grade: number;
  backgroundUrl?: string;
}

export default function CardArtHero({ imageUrl, cardName, grader, grade, backgroundUrl }: CardArtHeroProps) {
  return (
    <div className="flex-1 flex items-center justify-center w-full mb-4 min-h-0 max-h-[65%] relative">
      {/* Background: AI-generated or blurred card image */}
      {backgroundUrl ? (
        <img
          src={backgroundUrl}
          alt=""
          aria-hidden
          className="absolute inset-0 w-full h-full object-cover"
          crossOrigin="anonymous"
          style={{ filter: 'brightness(0.5)' }}
        />
      ) : imageUrl ? (
        <img
          src={imageUrl}
          alt=""
          aria-hidden
          className="absolute inset-0 w-full h-full object-cover"
          crossOrigin="anonymous"
          style={{ filter: 'blur(30px) brightness(0.4)', transform: 'scale(1.2)' }}
        />
      ) : null}

      {/* Card image — centered, clean */}
      <div className="relative z-[2] flex items-center justify-center w-full h-full">
        {imageUrl ? (
          <img
            src={imageUrl}
            alt={cardName}
            className="max-h-full max-w-[58%] object-contain rounded-lg shadow-2xl"
            crossOrigin="anonymous"
            style={{ filter: 'drop-shadow(0 25px 60px rgba(0,0,0,0.5))' }}
          />
        ) : (
          <div className="w-48 h-64 bg-white/5 border border-white/10 rounded-lg flex items-center justify-center p-4">
            <span className="text-sm text-center text-white/50">{cardName}</span>
          </div>
        )}

        {/* Floating PSA grade badge */}
        {imageUrl && (
          <div className="absolute top-1/2 right-[5%] -translate-y-1/2 bg-black/70 backdrop-blur-sm border border-white/15 rounded-xl px-3 py-2 text-center">
            <div className="text-[8px] text-white/50 uppercase tracking-[1px]">{grader || 'PSA'}</div>
            <div className="text-[22px] font-extrabold text-white leading-none">{grade}</div>
            <div className="text-[7px] text-amber-400 mt-0.5">
              {grade === 10 ? 'GEM MT' : grade === 9 ? 'MINT' : grade === 8 ? 'NM-MT' : grade === 7 ? 'NM' : ''}
            </div>
          </div>
        )}
      </div>
    </div>
  );
}
