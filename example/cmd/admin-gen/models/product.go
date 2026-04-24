package models

import (
	"encoding/json"
	"fmt"
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"
)

type ProductStatus string

const (
	ProductStatusActive   ProductStatus = "active"
	ProductStatusInactive ProductStatus = "inactive"
)

func (s ProductStatus) Validate() error {
	switch s {
	case "", ProductStatusActive, ProductStatusInactive:
		return nil
	}
	return fmt.Errorf("invalid status %q (allowed: active, inactive)", s)
}

//admin:generate
//admin:action name=purge destructive
type Product struct {
	ID        uuid.UUID       `json:"id" admin:"id" gorm:"primaryKey"`
	Name      string          `json:"name" admin:"filterable,sortable"`
	Price     float64         `json:"price" admin:"filterable,sortable"`
	Stock     int             `json:"stock" admin:"sortable"`
	Status    ProductStatus   `json:"status" admin:"filterable" gorm:"type:varchar(255)"`
	UserIDs   UUIDArray       `json:"user_ids" admin:"filterable" gorm:"type:uuid[]"`
	CreatedAt time.Time       `json:"created_at" admin:"readonly,sortable,filterable" gorm:"autoCreateTime"`
	UpdatedAt time.Time       `json:"updated_at" admin:"readonly,sortable,filterable" gorm:"autoUpdateTime"`
	DeletedAt gorm.DeletedAt  `json:"deleted_at" admin:"readonly,sortable,filterable"`
	Metadata  json.RawMessage `json:"metadata" gorm:"type:jsonb"`
}
