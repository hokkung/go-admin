package embedded_cross

import "gorm.io/gorm"

//admin:generate
type Post struct {
	ID    uint   `json:"id" admin:"id"`
	Title string `json:"title"`
	gorm.Model
}
