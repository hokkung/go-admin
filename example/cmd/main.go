package main

import (
	"encoding/json"
	"fmt"
	"log"
	"os"
	"time"

	"github.com/gofiber/fiber/v2"
	"github.com/google/uuid"
	"github.com/hokkung/go-admin/admin"
	gormstore "github.com/hokkung/go-admin/admin/storage/gorm"
	"github.com/hokkung/go-admin/example/cmd/model"

	"gorm.io/driver/postgres"
	"gorm.io/gorm"
)

type User struct {
	ID        uint           `json:"id" admin:"id" gorm:"primaryKey"`
	Name      string         `json:"name" admin:"filterable,sortable"`
	Email     string         `json:"email" admin:"filterable"`
	Age       int            `json:"age" admin:"filterable,sortable"`
	CreatedAt time.Time      `json:"created_at" admin:"readonly,sortable,filterable" gorm:"autoCreateTime"`
	UpdatedAt time.Time      `json:"updated_at" admin:"readonly,sortable,filterable" gorm:"autoUpdateTime"`
	DeletedAt gorm.DeletedAt `json:"deleted_at" admin:"readonly,sortable,filterable"`
}

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

type Product struct {
	ID        uuid.UUID       `json:"id" admin:"id" gorm:"primaryKey"`
	Name      string          `json:"name" admin:"filterable,sortable"`
	Price     float64         `json:"price" admin:"filterable,sortable"`
	Stock     int             `json:"stock" admin:"sortable"`
	Status    ProductStatus   `json:"status" admin:"filterable" gorm:"type:varchar(255)"`
	UserIDs   model.UUIDArray `json:"user_ids" admin:"filterable" gorm:"type:uuid[]"`
	CreatedAt time.Time       `json:"created_at" admin:"readonly,sortable,filterable" gorm:"autoCreateTime"`
	UpdatedAt time.Time       `json:"updated_at" admin:"readonly,sortable,filterable" gorm:"autoUpdateTime"`
	DeletedAt gorm.DeletedAt  `json:"deleted_at" admin:"readonly,sortable,filterable"`
	Metadata  json.RawMessage `json:"metadata" gorm:"type:jsonb"`
}

type Session struct {
	ID        uuid.UUID `json:"id" admin:"id"`
	UserEmail string    `json:"user_email" admin:"filterable,searchable"`
	IssuedAt  time.Time `json:"issued_at" admin:"readonly,sortable,filterable"`
}

func main() {
	dsn := os.Getenv("DATABASE_URL")
	if dsn == "" {
		dsn = "host=localhost user=admin password=admin dbname=go_admin port=5432 sslmode=disable"
	}
	db, err := gorm.Open(postgres.Open(dsn), &gorm.Config{})
	if err != nil {
		log.Fatal(err)
	}
	if err := db.AutoMigrate(&User{}, &Product{}); err != nil {
		log.Fatal(err)
	}

	gormStorage := gormstore.New(db)
	a := admin.New()
	a.MustRegister(&User{},
		admin.WithIDGenerator(admin.AutoIncrement()),
		admin.WithName("users"),
		admin.WithDisplayName("Users"),
		admin.WithStorage(gormStorage),
		admin.WithListConfig(admin.ListConfig{
			DefaultPageSize: 20,
			MaxPageSize:     100,
			DefaultSort:     []string{"-created_at"},
		}),
		admin.WithAction(admin.CustomAction{
			Name:        "resetPassword",
			DisplayName: "Reset Password",
			Input:       map[string]string{"user_id": "uint"},
			Handler: func(c *fiber.Ctx) error {
				return c.JSON(fiber.Map{"ok": true, "message": "reset link emailed"})
			},
		}),
		admin.WithAction(admin.CustomAction{
			Name:        "suspend",
			DisplayName: "Suspend User",
			Input:       map[string]string{"user_id": "uint", "reason": "string"},
			Destructive: true,
			Handler: func(c *fiber.Ctx) error {
				return c.JSON(fiber.Map{"ok": true, "suspended": true})
			},
		}),
	)
	a.MustRegister(
		&Product{},
		admin.WithStorage(gormStorage),
		admin.WithIDGenerator(admin.AutoUUID()),
	)
	a.MustRegister(
		&Session{},
		admin.WithStorage(gormStorage),
		admin.WithIDGenerator(admin.AutoUUID()),
	)

	app := fiber.New(fiber.Config{DisableStartupMessage: true})
	adminGroup := app.Group("api/v1/admin")
	a.Mount(adminGroup)

	addr := os.Getenv("ADDR")
	if addr == "" {
		addr = ":8089"
	}
	log.Printf("gorm admin listening on %s", addr)
	if err := app.Listen(addr); err != nil {
		log.Fatal(err)
	}
}
