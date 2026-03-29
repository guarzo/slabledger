package picks

import "github.com/guarzo/slabledger/internal/domain/errors"

const (
	ErrCodePicksAlreadyExist     errors.ErrorCode = "ERR_PICKS_ALREADY_EXIST"
	ErrCodeWatchlistItemNotFound errors.ErrorCode = "ERR_WATCHLIST_ITEM_NOT_FOUND"
	ErrCodeWatchlistDuplicate    errors.ErrorCode = "ERR_WATCHLIST_DUPLICATE"
	ErrCodeLLMFailure            errors.ErrorCode = "ERR_LLM_FAILURE"
)

var (
	ErrPicksAlreadyExist     = errors.NewAppError(ErrCodePicksAlreadyExist, "picks already exist for this date")
	ErrWatchlistItemNotFound = errors.NewAppError(ErrCodeWatchlistItemNotFound, "watchlist item not found")
	ErrWatchlistDuplicate    = errors.NewAppError(ErrCodeWatchlistDuplicate, "card already on watchlist")
	ErrLLMFailure            = errors.NewAppError(ErrCodeLLMFailure, "LLM generation failed")
)
