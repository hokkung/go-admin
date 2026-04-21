package gormstore_test

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/glebarez/sqlite"
	"github.com/gofiber/fiber/v2"
	"github.com/hokkung/go-admin/admin"
	gormstore "github.com/hokkung/go-admin/admin/storage/gorm"
	"gorm.io/gorm"
)

type widget struct {
	ID    uint   `json:"id" admin:"id" gorm:"primaryKey"`
	Name  string `json:"name" admin:"filterable,sortable"`
	Price int    `json:"price" admin:"filterable,sortable"`
}

// setup boots a fresh admin backed by gormstore on an isolated in-memory
// sqlite DB, so each test gets a clean database (Given: empty store).
func setup(t *testing.T) *fiber.App {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	if err := db.AutoMigrate(&widget{}); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	a := admin.New()
	a.MustRegister(&widget{}, admin.WithStorage(gormstore.New(db)))
	app := fiber.New(fiber.Config{DisableStartupMessage: true})
	a.Mount(app)
	return app
}

func post(t *testing.T, app *fiber.App, path string, body any) (int, map[string]any) {
	t.Helper()
	var buf bytes.Buffer
	if body != nil {
		_ = json.NewEncoder(&buf).Encode(body)
	}
	req := httptest.NewRequest(http.MethodPost, "/"+path, &buf)
	req.Header.Set("Content-Type", "application/json")
	resp, err := app.Test(req, -1)
	if err != nil {
		t.Fatalf("%s: %v", path, err)
	}
	defer resp.Body.Close()
	raw, _ := io.ReadAll(resp.Body)
	if len(raw) == 0 {
		return resp.StatusCode, nil
	}
	var out map[string]any
	if err := json.Unmarshal(raw, &out); err != nil {
		t.Fatalf("decode %s %q: %v", path, string(raw), err)
	}
	return resp.StatusCode, out
}

// seedWidget inserts a widget via the API and returns its generated id.
func seedWidget(t *testing.T, app *fiber.App, attrs map[string]any) float64 {
	t.Helper()
	status, body := post(t, app, "widget.create", attrs)
	if status != 200 {
		t.Fatalf("seedWidget: %d %v", status, body)
	}
	return body["id"].(float64)
}

// --- CRUD --------------------------------------------------------------

func TestGormStorageCRUD(t *testing.T) {
	t.Run("create persists the row and returns the DB-assigned id", func(t *testing.T) {
		// Given an empty widgets table
		app := setup(t)

		// When the client creates a widget without supplying an id
		status, body := post(t, app, "widget.create", map[string]any{"name": "gear", "price": 100})

		// Then the DB auto-increments the id and the row is returned
		if status != 200 {
			t.Fatalf("create: %d %v", status, body)
		}
		if id, ok := body["id"].(float64); !ok || id == 0 {
			t.Fatalf("create did not return an id: %v", body)
		}
	})

	t.Run("get returns a previously created row", func(t *testing.T) {
		// Given a widget exists
		app := setup(t)
		id := seedWidget(t, app, map[string]any{"name": "gear", "price": 100})

		// When the client fetches it by id
		status, body := post(t, app, "widget.get", map[string]any{"id": id})

		// Then the row comes back intact
		if status != 200 || body["name"] != "gear" {
			t.Fatalf("get: %d %v", status, body)
		}
	})

	t.Run("update merges a partial payload via GORM Save", func(t *testing.T) {
		// Given a widget exists with name=gear, price=100
		app := setup(t)
		id := seedWidget(t, app, map[string]any{"name": "gear", "price": 100})

		// When the client updates only the price
		status, body := post(t, app, "widget.update", map[string]any{"id": id, "price": 250})

		// Then price changes and name is preserved
		if status != 200 {
			t.Fatalf("update: %d %v", status, body)
		}
		if body["price"].(float64) != 250 || body["name"] != "gear" {
			t.Fatalf("merge did not preserve name: %v", body)
		}
	})

	t.Run("list respects filter and sort at the handler layer", func(t *testing.T) {
		// Given three widgets with distinct prices
		app := setup(t)
		for _, w := range []map[string]any{
			{"name": "gear", "price": 100},
			{"name": "bolt", "price": 5},
			{"name": "nut", "price": 3},
		} {
			if s, b := post(t, app, "widget.create", w); s != 200 {
				t.Fatalf("seed: %d %v", s, b)
			}
		}

		// When the client requests widgets cheaper than 100, sorted asc
		_, body := post(t, app, "widget.list", map[string]any{
			"filters": []map[string]any{{"field": "price", "op": "lt", "value": 100}},
			"sort":    []map[string]any{{"field": "price", "order": "asc"}},
		})

		// Then only bolt and nut remain, ordered by price ascending
		items := body["items"].([]any)
		if len(items) != 2 || items[0].(map[string]any)["name"] != "nut" {
			t.Fatalf("list: %v", body)
		}
	})

	t.Run("delete removes the row and subsequent get returns NOT_FOUND", func(t *testing.T) {
		// Given a widget exists
		app := setup(t)
		id := seedWidget(t, app, map[string]any{"name": "gear", "price": 100})

		// When the client deletes it
		status, body := post(t, app, "widget.delete", map[string]any{"id": id})

		// Then deletion is confirmed
		if status != 200 || body["deleted"] != true {
			t.Fatalf("delete: %d %v", status, body)
		}

		// And a subsequent get maps gorm.ErrRecordNotFound to the NOT_FOUND envelope
		status, body = post(t, app, "widget.get", map[string]any{"id": id})
		if status != 404 {
			t.Fatalf("post-delete get: %d %v", status, body)
		}
		if body["error"].(map[string]any)["code"] != "NOT_FOUND" {
			t.Fatalf("expected NOT_FOUND, got %v", body)
		}
	})
}

// --- Not-found paths ----------------------------------------------------

func TestGormStorageNotFound(t *testing.T) {
	t.Run("get on an unknown id returns NOT_FOUND", func(t *testing.T) {
		// Given an empty widgets table
		app := setup(t)

		// When the client fetches a non-existent id
		status, body := post(t, app, "widget.get", map[string]any{"id": 9999})

		// Then the server returns 404 with the canonical error envelope
		if status != 404 {
			t.Fatalf("expected 404, got %d %v", status, body)
		}
		if body["error"].(map[string]any)["code"] != "NOT_FOUND" {
			t.Fatalf("expected NOT_FOUND, got %v", body)
		}
	})

	t.Run("delete on an unknown id returns NOT_FOUND", func(t *testing.T) {
		// Given an empty widgets table
		app := setup(t)

		// When the client deletes a non-existent id
		status, body := post(t, app, "widget.delete", map[string]any{"id": 9999})

		// Then the server returns 404 (GORM reports 0 rows affected)
		if status != 404 {
			t.Fatalf("expected 404 on delete, got %d %v", status, body)
		}
	})
}
