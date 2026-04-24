package full

import "github.com/google/uuid"

//admin:generate
type Session struct {
	ID     uuid.UUID `json:"id" admin:"id"`
	UserID uint      `json:"user_id" admin:"filterable"`
	Timestamps
}
