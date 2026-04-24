// Package models holds the example project's domain types. Structs
// annotated with //admin:generate get handler + routing code emitted into
// ../admin by running `admin-gen -in . -out ../admin` from this dir.
package models

import "time"

//admin:generate
type User struct {
	ID        uint      `json:"id" admin:"id,sortable" gorm:"primaryKey"`
	Name      string    `json:"name" admin:"filterable,sortable,required"`
	Email     string    `json:"email" admin:"filterable,required"`
	Age       int       `json:"age" admin:"filterable,sortable"`
	DOB       *int      `json:"dob" admin:"filterable,sortable"`
	CreatedAt time.Time `json:"created_at" admin:"readonly,sortable,filterable" gorm:"autoCreateTime"`
}

type UserProfile struct {
	ID        uint      `json:"id" admin:"id,sortable" gorm:"primaryKey"`
	UserID    uint      `json:"user_id" admin:"filterable,required"`
	FirstName string    `json:"first_name" admin:"filterable,required"`
	LastName  string    `json:"last_name" admin:"filterable,required"`
	CreatedAt time.Time `json:"created_at" admin:"readonly,sortable,filterable" gorm:"autoCreateTime"`
}
