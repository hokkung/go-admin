// Example wiring for admin-gen-generated code. The routes emitted under
// ./admin/ are mounted under /api/v1/admin so you can POST
// /api/v1/admin/user.create from a client.
package main

import (
	"log"
	"os"

	"github.com/gofiber/fiber/v2"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"

	"github.com/hokkung/go-admin/example/cmd/admin-gen/admin"
	"github.com/hokkung/go-admin/example/cmd/admin-gen/models"
)

func main() {
	dsn := os.Getenv("DATABASE_URL")
	if dsn == "" {
		dsn = "host=localhost user=admin password=admin dbname=go_admin port=5432 sslmode=disable"
	}
	db, err := gorm.Open(postgres.Open(dsn), &gorm.Config{})
	if err != nil {
		log.Fatal(err)
	}
	if err := db.AutoMigrate(&models.User{}, &models.Session{}); err != nil {
		log.Fatal(err)
	}

	app := fiber.New(fiber.Config{DisableStartupMessage: true})
	a := admin.Register(db)
	a.Product.OnAction("purge", func(c *fiber.Ctx) error {
		// your business logic
		return c.JSON(fiber.Map{"ok": true})
	})
	a.Mount(app.Group("/api/v1/admin"))

	addr := os.Getenv("ADDR")
	if addr == "" {
		addr = ":8089"
	}
	log.Printf("admin-gen example listening on %s", addr)
	log.Fatal(app.Listen(addr))
}
