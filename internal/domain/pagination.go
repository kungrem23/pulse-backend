package domain

import "time"

// PagePosition is the exclusive keyset boundary for descending (created_at, id).
// For feed it refers to users; for match lists it refers to matches.
type PagePosition struct {
	CreatedAt time.Time
	ID        string
}

// PageQuery is validated by the application before calling a repository.
// After == nil means the first page. Limit is in [1, 100].
type PageQuery struct {
	After *PagePosition
	Limit int
}

// PageResult contains at most PageQuery.Limit items. Repositories look ahead
// one row to set HasMore; that extra row is not included in Items.
// Applications encode the last returned item's position only when HasMore.
type PageResult[T any] struct {
	Items   []T
	HasMore bool
}
