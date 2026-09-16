package mocks

import (
	"context"
	"database/sql"
)

// ShowPrepQueryHook delegates to real PostgreSQL while tests interleave a write
// between evidence lookup statements. It does not fabricate rows or SQL results.
type ShowPrepQueryHook struct {
	*sql.DB
	BeforeQuery func()
}

func (q *ShowPrepQueryHook) QueryContext(ctx context.Context, query string, args ...any) (*sql.Rows, error) {
	if q.BeforeQuery != nil {
		q.BeforeQuery()
	}
	return q.DB.QueryContext(ctx, query, args...)
}
