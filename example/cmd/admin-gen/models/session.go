package models

import (
	"time"

	"github.com/google/uuid"
)

//admin:generate default_page_size=50 max_page_size=200 default_sort="issued_at:desc"
type Session struct {
	ID        uuid.UUID `json:"id" admin:"id" gorm:"primaryKey;type:uuid"`
	UserEmail string    `json:"user_email" admin:"filterable,searchable"`
	IssuedAt  time.Time `json:"issued_at" admin:"readonly,sortable,filterable" gorm:"autoCreateTime"`
}
