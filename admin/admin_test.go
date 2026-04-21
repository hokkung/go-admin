package admin_test

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gofiber/fiber/v2"
	"github.com/google/uuid"
	"github.com/hokkung/go-admin/admin"
)

// --- fixtures -----------------------------------------------------------

type user struct {
	ID    uint   `json:"id" admin:"id,sortable"`
	Name  string `json:"name" admin:"filterable,sortable,searchable"`
	Email string `json:"email" admin:"filterable"`
	Age   int    `json:"age" admin:"filterable,sortable"`
	Role  string `json:"role" admin:"filterable" enum:"user,admin"`
}

type session struct {
	ID        uuid.UUID `json:"id" admin:"id"`
	UserEmail string    `json:"user_email" admin:"filterable"`
}

// status demonstrates field-level Validator enforcement.
type status string

func (s status) Validate() error {
	switch s {
	case "", "active", "inactive":
		return nil
	}
	return fmt.Errorf("invalid status %q", s)
}

type product struct {
	ID     uint   `json:"id" admin:"id"`
	Name   string `json:"name" admin:"filterable,sortable"`
	Status status `json:"status"`
}

// --- harness ------------------------------------------------------------

type testHarness struct {
	t   *testing.T
	app *fiber.App
}

// newHarness boots a fresh admin with three entities registered and returns
// a fiber app wired up for in-process HTTP testing. Each test gets its own
// harness so state does not leak between scenarios.
func newHarness(t *testing.T) *testHarness {
	t.Helper()
	a := admin.New()
	a.MustRegister(&user{}, admin.WithIDGenerator(admin.AutoIncrement()))
	a.MustRegister(&session{}, admin.WithIDGenerator(admin.AutoUUID()))
	a.MustRegister(&product{}, admin.WithIDGenerator(admin.AutoIncrement()),
		admin.WithAction(admin.CustomAction{
			Name:        "archive",
			DisplayName: "Archive",
			Destructive: true,
			Handler: func(c *fiber.Ctx) error {
				return c.JSON(fiber.Map{"archived": true})
			},
		}),
	)
	app := fiber.New(fiber.Config{DisableStartupMessage: true})
	a.Mount(app)
	return &testHarness{t: t, app: app}
}

func (h *testHarness) do(path string, body any) (int, map[string]any) {
	h.t.Helper()
	var buf bytes.Buffer
	if body != nil {
		if err := json.NewEncoder(&buf).Encode(body); err != nil {
			h.t.Fatalf("encode body: %v", err)
		}
	}
	req := httptest.NewRequest(http.MethodPost, "/"+path, &buf)
	req.Header.Set("Content-Type", "application/json")
	resp, err := h.app.Test(req, -1)
	if err != nil {
		h.t.Fatalf("app.Test(%s): %v", path, err)
	}
	defer resp.Body.Close()
	raw, _ := io.ReadAll(resp.Body)
	if len(raw) == 0 {
		return resp.StatusCode, nil
	}
	var out map[string]any
	if err := json.Unmarshal(raw, &out); err != nil {
		h.t.Fatalf("decode %s response %q: %v", path, string(raw), err)
	}
	return resp.StatusCode, out
}

// seedUser creates a user via the API and returns its generated id. Used as
// "Given" setup in tests that exercise get/update/delete/list scenarios.
func (h *testHarness) seedUser(attrs map[string]any) float64 {
	h.t.Helper()
	status, body := h.do("user.create", attrs)
	if status != 200 {
		h.t.Fatalf("seedUser: status=%d body=%v", status, body)
	}
	return body["id"].(float64)
}

// --- CRUD ---------------------------------------------------------------

func TestCRUD(t *testing.T) {
	t.Run("create returns the entity with a generated id", func(t *testing.T) {
		// Given a fresh admin with the user entity registered
		h := newHarness(t)
		payload := map[string]any{"name": "Alice", "email": "a@x.com", "age": 30, "role": "user"}

		// When the client POSTs user.create
		status, body := h.do("user.create", payload)

		// Then the response is 200 with a non-zero id and the submitted fields
		if status != 200 {
			t.Fatalf("status=%d body=%v", status, body)
		}
		if id, ok := body["id"].(float64); !ok || id == 0 {
			t.Fatalf("expected non-zero id, got %v", body["id"])
		}
		if body["name"] != "Alice" {
			t.Fatalf("name=%v", body["name"])
		}
	})

	t.Run("get returns a previously created entity", func(t *testing.T) {
		// Given a user exists with known attributes
		h := newHarness(t)
		id := h.seedUser(map[string]any{"name": "Alice", "email": "a@x.com", "age": 30, "role": "user"})

		// When the client POSTs user.get with that id
		status, body := h.do("user.get", map[string]any{"id": id})

		// Then the response echoes the stored user
		if status != 200 || body["name"] != "Alice" {
			t.Fatalf("status=%d body=%v", status, body)
		}
	})

	t.Run("update merges a partial payload", func(t *testing.T) {
		// Given an existing user
		h := newHarness(t)
		id := h.seedUser(map[string]any{"name": "Alice", "email": "a@x.com", "age": 30, "role": "user"})

		// When the client updates only the age
		status, body := h.do("user.update", map[string]any{"id": id, "age": 31})

		// Then age is updated and name is preserved (read-merge-write)
		if status != 200 {
			t.Fatalf("status=%d body=%v", status, body)
		}
		if body["age"].(float64) != 31 {
			t.Fatalf("age not updated: %v", body["age"])
		}
		if body["name"] != "Alice" {
			t.Fatalf("name should be preserved, got %v", body["name"])
		}
	})

	t.Run("delete removes the entity", func(t *testing.T) {
		// Given an existing user
		h := newHarness(t)
		id := h.seedUser(map[string]any{"name": "Alice", "email": "a@x.com", "age": 30, "role": "user"})

		// When the client POSTs user.delete
		status, body := h.do("user.delete", map[string]any{"id": id})

		// Then the server confirms deletion
		if status != 200 || body["deleted"] != true {
			t.Fatalf("delete: status=%d body=%v", status, body)
		}
	})

	t.Run("get after delete returns 404 NOT_FOUND", func(t *testing.T) {
		// Given a user that has been deleted
		h := newHarness(t)
		id := h.seedUser(map[string]any{"name": "Alice", "email": "a@x.com", "age": 30, "role": "user"})
		if s, b := h.do("user.delete", map[string]any{"id": id}); s != 200 {
			t.Fatalf("precondition: delete failed %d %v", s, b)
		}

		// When the client tries to fetch it
		status, body := h.do("user.get", map[string]any{"id": id})

		// Then the canonical NOT_FOUND envelope is returned
		if status != 404 {
			t.Fatalf("status=%d body=%v", status, body)
		}
		requireErrorCode(t, body, "NOT_FOUND")
	})
}

// --- List ---------------------------------------------------------------

func TestList(t *testing.T) {
	// Given three users with distinct ages and names, reused across subtests
	seed := func(h *testHarness) {
		for _, u := range []map[string]any{
			{"name": "Alice", "email": "a@x.com", "age": 30, "role": "user"},
			{"name": "Bob", "email": "b@x.com", "age": 25, "role": "user"},
			{"name": "Carol", "email": "c@x.com", "age": 40, "role": "admin"},
		} {
			if s, b := h.do("user.create", u); s != 200 {
				t.Fatalf("seed %v: %d %v", u, s, b)
			}
		}
	}

	t.Run("filter gt with sort desc returns only matching rows in order", func(t *testing.T) {
		// Given
		h := newHarness(t)
		seed(h)

		// When the client lists users with age > 26, sorted desc
		_, body := h.do("user.list", map[string]any{
			"filters": []map[string]any{{"field": "age", "op": "gt", "value": 26}},
			"sort":    []map[string]any{{"field": "age", "order": "desc"}},
		})

		// Then exactly Alice (30) and Carol (40) come back, Carol first
		items := body["items"].([]any)
		if len(items) != 2 {
			t.Fatalf("want 2 items, got %d", len(items))
		}
		if items[0].(map[string]any)["name"] != "Carol" {
			t.Fatalf("expected Carol first, got %v", items[0])
		}
	})

	t.Run("contains op narrows by substring", func(t *testing.T) {
		// Given
		h := newHarness(t)
		seed(h)

		// When the client filters name contains "ar"
		_, body := h.do("user.list", map[string]any{
			"filters": []map[string]any{{"field": "name", "op": "contains", "value": "ar"}},
		})

		// Then only Carol is returned
		items := body["items"].([]any)
		if len(items) != 1 || items[0].(map[string]any)["name"] != "Carol" {
			t.Fatalf("contains: got %v", items)
		}
	})

	t.Run("pagination slices the result set", func(t *testing.T) {
		// Given
		h := newHarness(t)
		seed(h)

		// When the client requests page 2 with page_size 2, sorted by name asc
		_, body := h.do("user.list", map[string]any{
			"sort":      []map[string]any{{"field": "name", "order": "asc"}},
			"page":      2,
			"page_size": 2,
		})

		// Then total reflects all rows, page 2 holds the single remaining row (Carol)
		if body["total"].(float64) != 3 || body["page"].(float64) != 2 {
			t.Fatalf("pagination meta: %v", body)
		}
		items := body["items"].([]any)
		if len(items) != 1 || items[0].(map[string]any)["name"] != "Carol" {
			t.Fatalf("pagination page 2: %v", items)
		}
	})

	t.Run("filter on non-filterable field is rejected with BAD_REQUEST", func(t *testing.T) {
		// Given
		h := newHarness(t)
		seed(h)

		// When the client tries to filter on id (not tagged filterable)
		status, body := h.do("user.list", map[string]any{
			"filters": []map[string]any{{"field": "id", "op": "eq", "value": 1}},
		})

		// Then the server rejects with BAD_REQUEST
		if status != 400 {
			t.Fatalf("expected 400, got %d %v", status, body)
		}
		requireErrorCode(t, body, "BAD_REQUEST")
	})
}

// --- Validation ---------------------------------------------------------

func TestValidation(t *testing.T) {
	t.Run("enum tag rejects values outside the declared set", func(t *testing.T) {
		// Given the user entity has role enum:"user,admin"
		h := newHarness(t)

		// When creating a user with role=super
		status, body := h.do("user.create", map[string]any{"name": "x", "email": "x@x", "role": "super"})

		// Then the server returns VALIDATION_FAILED
		if status != 400 {
			t.Fatalf("expected 400, got %d %v", status, body)
		}
		requireErrorCode(t, body, "VALIDATION_FAILED")
	})

	t.Run("enum tag allows empty string as unset", func(t *testing.T) {
		// Given
		h := newHarness(t)

		// When creating a user without providing a role
		status, body := h.do("user.create", map[string]any{"name": "y", "email": "y@x"})

		// Then the create succeeds
		if status != 200 {
			t.Fatalf("empty enum rejected: %d %v", status, body)
		}
	})

	t.Run("Validator interface rejects invalid field values", func(t *testing.T) {
		// Given product.status is a Validator-implementing type
		h := newHarness(t)

		// When creating a product with an unsupported status
		status, body := h.do("product.create", map[string]any{"name": "p", "status": "frozen"})

		// Then the server returns VALIDATION_FAILED
		if status != 400 {
			t.Fatalf("expected 400, got %d %v", status, body)
		}
		requireErrorCode(t, body, "VALIDATION_FAILED")
	})

	t.Run("validators also run on update", func(t *testing.T) {
		// Given a product exists with a valid status
		h := newHarness(t)
		_, body := h.do("product.create", map[string]any{"name": "p", "status": "active"})
		id := body["id"]

		// When the client tries to update it to an invalid status
		status, body := h.do("product.update", map[string]any{"id": id, "status": "frozen"})

		// Then the server returns VALIDATION_FAILED
		if status != 400 {
			t.Fatalf("expected 400 on update, got %d %v", status, body)
		}
		requireErrorCode(t, body, "VALIDATION_FAILED")
	})
}

// --- ID generation ------------------------------------------------------

func TestUUIDIDGen(t *testing.T) {
	t.Run("AutoUUID mints a valid UUID when id is omitted", func(t *testing.T) {
		// Given session is registered with AutoUUID
		h := newHarness(t)

		// When creating a session without providing an id
		status, body := h.do("session.create", map[string]any{"user_email": "a@x.com"})

		// Then the response contains a parseable uuid.UUID string
		if status != 200 {
			t.Fatalf("create: %d %v", status, body)
		}
		idStr, ok := body["id"].(string)
		if !ok {
			t.Fatalf("id not a string: %v", body["id"])
		}
		if _, err := uuid.Parse(idStr); err != nil {
			t.Fatalf("id %q is not a valid uuid: %v", idStr, err)
		}
	})

	t.Run("UUID id round-trips through get", func(t *testing.T) {
		// Given a session exists under a generated UUID
		h := newHarness(t)
		_, body := h.do("session.create", map[string]any{"user_email": "a@x.com"})
		idStr := body["id"].(string)

		// When the client fetches it by that id
		status, body := h.do("session.get", map[string]any{"id": idStr})

		// Then the server returns the same entity (exercises the goType-based extractID)
		if status != 200 || body["id"] != idStr {
			t.Fatalf("uuid round-trip failed: %d %v", status, body)
		}
	})

	t.Run("an explicit id is respected and the generator is skipped", func(t *testing.T) {
		// Given the client supplies a specific UUID
		h := newHarness(t)
		explicit := "11111111-1111-1111-1111-111111111111"

		// When creating with that id
		_, body := h.do("session.create", map[string]any{"id": explicit, "user_email": "b@x.com"})

		// Then the server stores and returns the supplied id unchanged
		if body["id"] != explicit {
			t.Fatalf("explicit id not respected: %v", body)
		}
	})

	t.Run("a malformed uuid produces a structured BAD_REQUEST", func(t *testing.T) {
		// Given
		h := newHarness(t)

		// When the client calls get with an unparseable id
		status, body := h.do("session.get", map[string]any{"id": "not-a-uuid"})

		// Then the server returns BAD_REQUEST (not a 500)
		if status != 400 {
			t.Fatalf("expected 400 for bad uuid, got %d %v", status, body)
		}
		requireErrorCode(t, body, "BAD_REQUEST")
	})
}

// --- Metadata & catalog -------------------------------------------------

func TestMetadataAndCatalog(t *testing.T) {
	t.Run("entity.metadata describes the schema including enum options", func(t *testing.T) {
		// Given a fresh admin
		h := newHarness(t)

		// When the client requests user.metadata
		_, body := h.do("user.metadata", nil)

		// Then the schema contains the name, primary key and typed field list
		data := body["data"].(map[string]any)
		if data["name"] != "user" {
			t.Fatalf("name: %v", data["name"])
		}
		if data["primary_key"] != "id" {
			t.Fatalf("primary_key: %v", data["primary_key"])
		}
		var role map[string]any
		for _, f := range data["fields"].([]any) {
			m := f.(map[string]any)
			if m["name"] == "role" {
				role = m
			}
		}
		if role == nil || role["type"] != "enum" {
			t.Fatalf("role field missing or not enum: %v", role)
		}
		if len(role["options"].([]any)) != 2 {
			t.Fatalf("expected 2 enum options, got %v", role["options"])
		}
	})

	t.Run("admin.entities returns the full catalog with paths", func(t *testing.T) {
		// Given three entities are registered
		h := newHarness(t)

		// When the client requests admin.entities
		_, body := h.do("admin.entities", nil)

		// Then every entity is listed with a non-empty path
		entities := body["data"].(map[string]any)["entities"].([]any)
		if len(entities) != 3 {
			t.Fatalf("expected 3 entities, got %d", len(entities))
		}
		for _, e := range entities {
			m := e.(map[string]any)
			if m["path"] == nil || m["path"] == "" {
				t.Fatalf("entity missing path: %v", m)
			}
		}
	})

	t.Run("custom actions surface in the entity metadata", func(t *testing.T) {
		// Given product has an "archive" custom action
		h := newHarness(t)

		// When the client requests product.metadata
		_, body := h.do("product.metadata", nil)

		// Then the archive action appears under actions.custom
		actions := body["data"].(map[string]any)["actions"].(map[string]any)
		custom := actions["custom"].([]any)
		if len(custom) != 1 || custom[0].(map[string]any)["name"] != "archive" {
			t.Fatalf("expected archive custom action, got %v", custom)
		}
	})
}

// --- Custom action dispatch --------------------------------------------

func TestCustomActionDispatch(t *testing.T) {
	// Given product has an "archive" action returning {"archived": true}
	h := newHarness(t)

	// When the client calls product.archive
	status, body := h.do("product.archive", nil)

	// Then the registered handler runs and its response is forwarded
	if status != 200 || body["archived"] != true {
		t.Fatalf("custom action: %d %v", status, body)
	}
}

// --- Dispatch errors (table-driven) ------------------------------------

func TestDispatchErrors(t *testing.T) {
	// Given — table of malformed or mis-routed requests; each row becomes a
	// subtest with the same Given/When/Then shape (single shared harness).
	h := newHarness(t)
	cases := []struct {
		name string
		path string
		want string
		code int
	}{
		{"unknown entity", "ghost.list", "INVALID_ENTITY", 404},
		{"unknown action", "user.frobnicate", "INVALID_ACTION", 404},
		{"unknown system action", "admin.frobnicate", "INVALID_ACTION", 404},
		{"missing dot in path", "userlist", "BAD_REQUEST", 400},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			// When the client POSTs the malformed path
			status, body := h.do(tc.path, nil)

			// Then the server returns the expected status and canonical error code
			if status != tc.code {
				t.Fatalf("status=%d want=%d body=%v", status, tc.code, body)
			}
			requireErrorCode(t, body, tc.want)
		})
	}
}

// --- Registration guardrails -------------------------------------------

func TestRegister(t *testing.T) {
	t.Run("registering with the reserved name 'admin' is rejected", func(t *testing.T) {
		// Given a model that would clash with the framework's system namespace
		type admins struct {
			ID uint `json:"id" admin:"id"`
		}
		a := admin.New()

		// When the caller tries to register it under the reserved name
		err := a.Register(&admins{}, admin.WithName("admin"))

		// Then registration fails
		if err == nil {
			t.Fatal("expected error when registering reserved name")
		}
	})

	t.Run("custom action name colliding with a standard action is rejected", func(t *testing.T) {
		// Given a model with a custom action named "create"
		type ticket struct {
			ID uint `json:"id" admin:"id"`
		}
		a := admin.New()

		// When the caller tries to register it
		err := a.Register(&ticket{},
			admin.WithAction(admin.CustomAction{
				Name:    "create",
				Handler: func(c *fiber.Ctx) error { return nil },
			}),
		)

		// Then registration fails
		if err == nil {
			t.Fatal("expected error when custom action clashes with standard action")
		}
	})
}

// --- helpers ------------------------------------------------------------

func requireErrorCode(t *testing.T, body map[string]any, want string) {
	t.Helper()
	errObj, ok := body["error"].(map[string]any)
	if !ok {
		t.Fatalf("no error envelope in body: %v", body)
	}
	if errObj["code"] != want {
		t.Fatalf("expected code=%q, got %v", want, errObj["code"])
	}
	if errObj["message"] == nil || errObj["message"] == "" {
		t.Fatalf("expected non-empty message, got %v", errObj)
	}
}
