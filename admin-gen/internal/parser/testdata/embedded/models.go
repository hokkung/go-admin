package embedded

import "time"

type Timestamps struct {
	CreatedAt time.Time `json:"created_at" admin:"sortable"`
	UpdatedAt time.Time `json:"updated_at" admin:"sortable"`
}

//admin:generate
type Post struct {
	ID    uint   `json:"id" admin:"id"`
	Title string `json:"title" admin:"filterable"`
	Timestamps
}
