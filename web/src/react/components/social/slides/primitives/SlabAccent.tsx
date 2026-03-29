import type { PostType } from '../../../../../types/social';
import { getTheme } from './theme';
import { corsUrl } from './corsUrl';

interface SlabAccentProps {
  imageUrl: string;
  cardName: string;
  postType: PostType;
}

export default function SlabAccent({ imageUrl, cardName, postType }: SlabAccentProps) {
  const theme = getTheme(postType);
  return (
    <div className="flex-1 flex items-center justify-center w-full mb-4 min-h-0 max-h-[60%]">
      {imageUrl ? (
        <img
          src={corsUrl(imageUrl)}
          alt={cardName}
          className="max-h-full max-w-[55%] object-contain rounded-lg"
          crossOrigin="anonymous"
          style={{
            transform: 'perspective(800px) rotateY(-8deg) rotateX(3deg)',
            filter: `drop-shadow(0 20px 40px rgba(0,0,0,0.5)) drop-shadow(0 0 30px ${theme.glowColor})`,
          }}
        />
      ) : (
        <div className="w-48 h-64 bg-white/5 border border-white/10 rounded-lg flex items-center justify-center p-4">
          <span className="text-sm text-center text-white/50">{cardName}</span>
        </div>
      )}
    </div>
  );
}
