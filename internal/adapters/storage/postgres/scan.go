package postgres

import (
	"context"
	"database/sql"
)

// scanRows iterates over rows, calling scanFn for each row, and handles
// Close/Err/context cancellation. This eliminates the repeated
// defer-close + for-next + context-check boilerplate across repositories.
func scanRows[T any](ctx context.Context, rows *sql.Rows, scanFn func(*sql.Rows) (T, error)) (result []T, err error) {
	defer func() {
		if cerr := rows.Close(); err == nil && cerr != nil {
			err = cerr
		}
	}()

	for rows.Next() {
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		default:
		}

		item, scanErr := scanFn(rows)
		if scanErr != nil {
			return nil, scanErr
		}
		result = append(result, item)
	}
	return result, rows.Err()
}
