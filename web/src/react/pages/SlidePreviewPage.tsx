import { useSearchParams } from 'react-router-dom';
import { useSocialPost } from '../queries/useSocialQueries';
import CoverSlide from '../components/social/slides/CoverSlide';
import CardSlide from '../components/social/slides/CardSlide';
import PokeballLoader from '../PokeballLoader';

/**
 * Bare-bones page for headless slide rendering by the Puppeteer sidecar.
 * No auth, no header, no chrome. Renders exactly one slide via [data-slide].
 *
 * Query params:
 *   postId     — UUID of the social post
 *   slideIndex — 0 = cover slide, N = card at index N-1
 */
export default function SlidePreviewPage() {
  const [params] = useSearchParams();
  const postId = params.get('postId') ?? '';
  const slideIndex = parseInt(params.get('slideIndex') ?? '0', 10);

  const { data: post, isLoading, isError } = useSocialPost(postId);

  if (!postId) {
    return <div className="text-red-500 p-4">Missing postId</div>;
  }
  if (isLoading) {
    return (
      <div className="flex items-center justify-center" style={{ width: 1080, height: 1080 }}>
        <PokeballLoader />
      </div>
    );
  }
  if (isError || !post) {
    return <div className="text-red-500 p-4">Post not found</div>;
  }

  if (isNaN(slideIndex) || slideIndex < 0) {
    return <div className="text-red-500 p-4">Invalid slideIndex</div>;
  }

  const isCover = slideIndex === 0;
  const card = isCover ? undefined : post.cards?.[slideIndex - 1];
  const totalSlides = (post.cards?.length ?? 0) + 1; // +1 for cover

  return (
    <div style={{ width: 1080, height: 1080, overflow: 'hidden' }}>
      {isCover ? (
        <CoverSlide
          postType={post.postType}
          coverTitle={post.coverTitle}
          cardCount={post.cardCount}
          psa10Count={post.cards?.filter((c) => c.gradeValue === 10).length ?? 0}
          cards={post.cards}
          backgroundUrls={post.backgroundUrls}
        />
      ) : card ? (
        <CardSlide
          card={card}
          postType={post.postType}
          slideIndex={slideIndex}
          totalSlides={totalSlides}
          backgroundUrl={post.backgroundUrls?.[slideIndex]}
        />
      ) : (
        <div className="text-red-500 p-4">Slide index out of range</div>
      )}
    </div>
  );
}
