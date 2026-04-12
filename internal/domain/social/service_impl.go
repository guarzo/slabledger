package social

import (
	"context"
	"sync"

	"github.com/guarzo/slabledger/internal/domain/ai"
	"github.com/guarzo/slabledger/internal/domain/observability"
)

const (
	defaultMinCards = 1
	defaultMaxCards = 9

	// Detection thresholds
	priceChangeThreshold = 0.15 // 15% price change for price movers
	hotDealThreshold     = 0.70 // buy cost < 70% of median = hot deal
	maxSnapshotAgeDays   = 7    // skip cards with snapshots older than this
	newArrivalsWindow    = 7    // days to look back for new arrivals
)

// placeholderCaption is set when AI caption generation fails.
// Posts with this caption must not be published.
const placeholderCaption = "(Caption generation failed — click Regenerate to try again)"

type service struct {
	repo          Repository
	llm           ai.LLMProvider
	publisher     Publisher
	tokenProvider InstagramTokenProvider
	logger        observability.Logger
	tracker       ai.AICallTracker
	imageGen      ai.ImageGenerator
	mediaStore    MediaStore
	imageQuality  string
	mediaDir      string
	baseURL       string
	minCards      int
	maxCards      int
	wg            sync.WaitGroup
}

// NewService creates a new social content service.
func NewService(repo Repository, opts ...ServiceOption) Service {
	s := &service{
		repo:     repo,
		minCards: defaultMinCards,
		maxCards: defaultMaxCards,
	}
	for _, opt := range opts {
		opt(s)
	}
	return s
}

var _ Service = (*service)(nil)

func (s *service) DetectAndGenerate(ctx context.Context) (int, error) {
	// Try LLM-powered generation first if available
	if s.llm != nil {
		created, err := s.llmGenerate(ctx)
		if err != nil {
			if s.logger != nil {
				s.logger.Warn(ctx, "social: LLM generation failed, falling back to rule-based detection — check AI health",
					observability.Err(err))
			}
		} else if created > 0 {
			return created, nil
		}
	}

	// Fallback: rule-based detection
	return s.ruleBasedGenerate(ctx)
}
