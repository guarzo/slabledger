package mocks

import (
	"context"
	"database/sql"
)

// ShowPrepQueryHook delegates to real PostgreSQL while tests observe SQL or
// interleave writes. It does not fabricate rows or SQL results.
type ShowPrepQueryHook struct {
	*sql.DB
	BeforeQuery  func()
	ObserveQuery func(string, []any)
}

func (q *ShowPrepQueryHook) QueryContext(ctx context.Context, query string, args ...any) (*sql.Rows, error) {
	if q.BeforeQuery != nil {
		q.BeforeQuery()
	}
	if q.ObserveQuery != nil {
		q.ObserveQuery(query, args)
	}
	return q.DB.QueryContext(ctx, query, args...)
}
