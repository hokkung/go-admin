package runtime

import "gorm.io/gorm"

// Filter is a single predicate in a list request.
type Filter struct {
	Field string `json:"field"`
	Op    string `json:"op"`
	Value any    `json:"value"`
}

// SortSpec is a single ORDER BY column.
type SortSpec struct {
	Field string `json:"field"`
	Order string `json:"order"`
}

// ListQuery mirrors the wire shape accepted by every <entity>.list endpoint.
type ListQuery struct {
	Filters  []Filter   `json:"filters"`
	Sort     []SortSpec `json:"sort"`
	Page     int        `json:"page"`
	PageSize int        `json:"page_size"`
}

// ListResult is the wire response shape for list endpoints. Generics keep
// the generated code typed — you get ListResult[User] in the handler, not
// a bag of interface{}.
type ListResult[T any] struct {
	Items    []T `json:"items"`
	Total    int `json:"total"`
	Page     int `json:"page"`
	PageSize int `json:"page_size"`
}

// ApplyGormFilter translates a Filter into the appropriate GORM Where
// clause. The column name comes from the caller — the generated handler
// passes an allowlisted string, so there is no SQL-injection surface here.
//
// If you need an operator that isn't in this switch, generated handlers
// can fall back to writing the clause inline; this helper is a convenience,
// not a bottleneck.
func ApplyGormFilter(tx *gorm.DB, col string, f Filter) *gorm.DB {
	switch f.Op {
	case "", "eq":
		return tx.Where(quoteIdent(col)+" = ?", f.Value)
	case "ne":
		return tx.Where(quoteIdent(col)+" <> ?", f.Value)
	case "lt":
		return tx.Where(quoteIdent(col)+" < ?", f.Value)
	case "lte":
		return tx.Where(quoteIdent(col)+" <= ?", f.Value)
	case "gt":
		return tx.Where(quoteIdent(col)+" > ?", f.Value)
	case "gte":
		return tx.Where(quoteIdent(col)+" >= ?", f.Value)
	case "contains":
		s, _ := f.Value.(string)
		return tx.Where(quoteIdent(col)+" LIKE ?", "%"+s+"%")
	case "in":
		return tx.Where(quoteIdent(col)+" IN ?", f.Value)
	}
	// Unknown op — return tx unchanged so the caller can decide how to react.
	return tx
}

// NormalizeQuery applies defaults for page/page_size. Generated handlers
// call this after decoding the request body so the rest of the flow can
// assume q.Page >= 1 and q.PageSize > 0.
func NormalizeQuery(q *ListQuery, defaultPageSize, maxPageSize int) {
	if q.PageSize <= 0 {
		q.PageSize = defaultPageSize
	}
	if maxPageSize > 0 && q.PageSize > maxPageSize {
		q.PageSize = maxPageSize
	}
	if q.Page <= 0 {
		q.Page = 1
	}
}

// quoteIdent wraps a column name in double quotes so reserved words
// (timestamp, user, etc.) are safe. Postgres and SQLite both accept
// double-quoted identifiers; MySQL users should swap to backticks.
func quoteIdent(name string) string {
	out := make([]byte, 0, len(name)+2)
	out = append(out, '"')
	for i := 0; i < len(name); i++ {
		if name[i] == '"' {
			out = append(out, '"', '"')
			continue
		}
		out = append(out, name[i])
	}
	return string(append(out, '"'))
}
