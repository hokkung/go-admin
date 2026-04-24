package uuid_id

import "github.com/google/uuid"

//admin:generate
type Session struct {
	ID    uuid.UUID `json:"id" admin:"id"`
	Email string    `json:"email"`
}
