package full

import "time"

// Timestamps is flattened into every entity that embeds it — exercises the
// same-package embed resolution path.
type Timestamps struct {
	CreatedAt time.Time `json:"created_at" admin:"readonly,sortable,filterable"`
	UpdatedAt time.Time `json:"updated_at" admin:"readonly,sortable,filterable"`
}
