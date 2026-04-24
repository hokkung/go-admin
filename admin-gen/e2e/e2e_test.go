// Package e2e drives the code admin-gen emits end-to-end: open sqlite
// in-memory, mount the example project's generated handlers onto a fiber
// app, and hit every declared endpoint with fiber.App.Test. The committed
// example/cmd/admin-gen/admin package is treated here as "the generator's
// output" — the generator-golden tests cover template correctness in
// isolation; this file covers what happens when that output actually runs.
package e2e

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/glebarez/sqlite"
	"github.com/gofiber/fiber/v2"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"

	genAdmin "github.com/hokkung/go-admin/example/cmd/admin-gen/admin"
	"github.com/hokkung/go-admin/example/cmd/admin-gen/models"
)

// testApp sets up a fiber App with the generated admin routes mounted at
// /admin, backed by a fresh sqlite in-memory database and AutoMigrated
// schemas for every example model. Returned to each test unwrapped so
// tests can attach custom actions / id generators before issuing requests.
func testApp(t *testing.T) (*fiber.App, *genAdmin.Admin, *gorm.DB) {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{
		Logger: logger.Default.LogMode(logger.Silent),
	})
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	if err := db.AutoMigrate(&models.User{}, &models.Session{}); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	a := genAdmin.Register(db)
	app := fiber.New(fiber.Config{DisableStartupMessage: true})
	a.Mount(app.Group("/admin"))
	return app, a, db
}

// do issues a POST to /admin/<path> with optional JSON body and returns
// the status and decoded body. Centralized so individual tests can focus
// on assertion rather than HTTP plumbing.
func do(t *testing.T, app *fiber.App, path string, body any) (int, map[string]any) {
	t.Helper()
	var buf io.Reader
	if body != nil {
		b, err := json.Marshal(body)
		if err != nil {
			t.Fatalf("marshal body: %v", err)
		}
		buf = bytes.NewReader(b)
	}
	req := httptest.NewRequest("POST", "/admin/"+path, buf)
	if buf != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	resp, err := app.Test(req)
	if err != nil {
		t.Fatalf("app.Test %s: %v", path, err)
	}
	rawBody, _ := io.ReadAll(resp.Body)
	out := map[string]any{}
	if len(rawBody) > 0 {
		if err := json.Unmarshal(rawBody, &out); err != nil {
			t.Fatalf("decode %s (status=%d body=%s): %v", path, resp.StatusCode, rawBody, err)
		}
	}
	return resp.StatusCode, out
}

// --- CRUD ---------------------------------------------------------------

func TestE2E_GivenValidCreateBody_WhenPostedToUserCreate_ThenReturnsEntityWithGeneratedID(t *testing.T) {
	app, _, _ := testApp(t)

	status, created := do(t, app, "user.create", map[string]any{
		"name":  "Alice",
		"email": "alice@example.com",
		"age":   30,
	})
	if status != http.StatusOK {
		t.Fatalf("create status = %d, body=%v", status, created)
	}
	idFloat, ok := created["id"].(float64)
	if !ok || idFloat == 0 {
		t.Fatalf("created.id = %v, want non-zero", created["id"])
	}

	status, got := do(t, app, "user.get", map[string]any{"id": int(idFloat)})
	if status != http.StatusOK {
		t.Fatalf("get status = %d body=%v", status, got)
	}
	if got["name"] != "Alice" {
		t.Fatalf("get.name = %v, want Alice", got["name"])
	}
}

func TestE2E_GivenNonExistentID_WhenPostedToUserGet_ThenReturns404WithErrorEnvelope(t *testing.T) {
	app, _, _ := testApp(t)
	status, body := do(t, app, "user.get", map[string]any{"id": 99999})
	if status != http.StatusNotFound {
		t.Fatalf("status = %d body=%v", status, body)
	}
	errObj, ok := body["error"].(map[string]any)
	if !ok || errObj["code"] != "NOT_FOUND" {
		t.Fatalf("error envelope shape wrong: %v", body)
	}
}

func TestE2E_GivenMalformedJSONBody_WhenPostedToUserCreate_ThenReturns400(t *testing.T) {
	app, _, _ := testApp(t)
	req := httptest.NewRequest("POST", "/admin/user.create", bytes.NewReader([]byte("{not json")))
	req.Header.Set("Content-Type", "application/json")
	resp, err := app.Test(req)
	if err != nil {
		t.Fatal(err)
	}
	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("status = %d", resp.StatusCode)
	}
}

func TestE2E_GivenPartialUpdateBody_WhenPostedToUserUpdate_ThenOtherFieldsArePreserved(t *testing.T) {
	app, _, _ := testApp(t)
	_, created := do(t, app, "user.create", map[string]any{"name": "Bob", "email": "b@x", "age": 25})
	id := int(created["id"].(float64))

	// Only supply age — the update must preserve name and email.
	status, updated := do(t, app, "user.update", map[string]any{"id": id, "age": 31})
	if status != http.StatusOK {
		t.Fatalf("update status = %d body=%v", status, updated)
	}
	if updated["name"] != "Bob" || updated["email"] != "b@x" {
		t.Fatalf("partial update clobbered untouched fields: %+v", updated)
	}
	if age, _ := updated["age"].(float64); age != 31 {
		t.Fatalf("age not updated: %v", updated["age"])
	}
}

func TestE2E_GivenUpdateBodyWithoutID_WhenPostedToUserUpdate_ThenReturns400(t *testing.T) {
	app, _, _ := testApp(t)
	status, _ := do(t, app, "user.update", map[string]any{"name": "x"})
	if status != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400", status)
	}
}

func TestE2E_GivenExistingID_WhenPostedToUserDeleteTwice_ThenFirstSucceedsSecondReturns404(t *testing.T) {
	app, _, _ := testApp(t)
	_, created := do(t, app, "user.create", map[string]any{"name": "C", "email": "c@x", "age": 40})
	id := int(created["id"].(float64))

	status, del := do(t, app, "user.delete", map[string]any{"id": id})
	if status != http.StatusOK || del["deleted"] != true {
		t.Fatalf("delete status=%d body=%v", status, del)
	}

	// A second delete on the same id should now 404.
	status, _ = do(t, app, "user.delete", map[string]any{"id": id})
	if status != http.StatusNotFound {
		t.Fatalf("second delete status = %d, want 404", status)
	}
}

// --- List: filter / sort / pagination ----------------------------------

func seedUsers(t *testing.T, app *fiber.App, names ...string) {
	t.Helper()
	for i, n := range names {
		_, body := do(t, app, "user.create", map[string]any{"name": n, "email": n + "@x", "age": i * 10})
		if _, ok := body["id"]; !ok {
			t.Fatalf("seed create failed for %q: %v", n, body)
		}
	}
}

func TestE2E_GivenPopulatedTableAndEmptyQuery_WhenPostedToUserList_ThenReturnsAllRowsWithTotal(t *testing.T) {
	app, _, _ := testApp(t)
	seedUsers(t, app, "Alice", "Bob", "Carol")

	status, body := do(t, app, "user.list", nil)
	if status != http.StatusOK {
		t.Fatalf("status = %d body=%v", status, body)
	}
	if total, _ := body["total"].(float64); int(total) != 3 {
		t.Fatalf("total = %v, want 3", body["total"])
	}
	items, _ := body["items"].([]any)
	if len(items) != 3 {
		t.Fatalf("items = %d, want 3", len(items))
	}
}

func TestE2E_GivenEqFilter_WhenPostedToUserList_ThenReturnsOnlyMatchingRows(t *testing.T) {
	app, _, _ := testApp(t)
	seedUsers(t, app, "Alice", "Bob")

	_, body := do(t, app, "user.list", map[string]any{
		"filters": []map[string]any{{"field": "name", "op": "eq", "value": "Alice"}},
	})
	if total, _ := body["total"].(float64); int(total) != 1 {
		t.Fatalf("eq filter returned %v results, want 1; body=%v", body["total"], body)
	}
}

func TestE2E_GivenContainsFilter_WhenPostedToUserList_ThenReturnsSubstringMatches(t *testing.T) {
	app, _, _ := testApp(t)
	seedUsers(t, app, "Alicia", "Alice", "Bob")

	_, body := do(t, app, "user.list", map[string]any{
		"filters": []map[string]any{{"field": "name", "op": "contains", "value": "Ali"}},
	})
	if total, _ := body["total"].(float64); int(total) != 2 {
		t.Fatalf("contains 'Ali' returned %v, want 2; body=%v", body["total"], body)
	}
}

func TestE2E_GivenFilterOnNonAllowlistedField_WhenPostedToUserList_ThenReturns400(t *testing.T) {
	app, _, _ := testApp(t)
	status, body := do(t, app, "user.list", map[string]any{
		"filters": []map[string]any{{"field": "password", "op": "eq", "value": "x"}},
	})
	if status != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400; body=%v", status, body)
	}
}

func TestE2E_GivenSortAscByName_WhenPostedToUserList_ThenReturnsRowsInAlphabeticalOrder(t *testing.T) {
	app, _, _ := testApp(t)
	seedUsers(t, app, "Charlie", "Alice", "Bob") // inserted out of order

	_, body := do(t, app, "user.list", map[string]any{
		"sort": []map[string]any{{"field": "name", "order": "asc"}},
	})
	items, _ := body["items"].([]any)
	if len(items) != 3 {
		t.Fatalf("items count = %d", len(items))
	}
	got := []string{}
	for _, it := range items {
		got = append(got, it.(map[string]any)["name"].(string))
	}
	want := []string{"Alice", "Bob", "Charlie"}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("sort asc got %v, want %v", got, want)
		}
	}
}

func TestE2E_GivenPaginationWithPageAndPageSize_WhenPostedToUserList_ThenReturnsOnlyThatSliceAndPreservesTotal(t *testing.T) {
	app, _, _ := testApp(t)
	seedUsers(t, app, "A", "B", "C", "D", "E")

	// page 1, size 2 — expect 2 items but total=5
	_, body := do(t, app, "user.list", map[string]any{"page": 1, "page_size": 2, "sort": []map[string]any{{"field": "name", "order": "asc"}}})
	items, _ := body["items"].([]any)
	if len(items) != 2 {
		t.Fatalf("page 1 len = %d", len(items))
	}
	if total, _ := body["total"].(float64); int(total) != 5 {
		t.Fatalf("total = %v", body["total"])
	}

	// page 3, size 2 — just "E"
	_, body = do(t, app, "user.list", map[string]any{"page": 3, "page_size": 2, "sort": []map[string]any{{"field": "name", "order": "asc"}}})
	items, _ = body["items"].([]any)
	if len(items) != 1 || items[0].(map[string]any)["name"] != "E" {
		t.Fatalf("page 3 items = %+v", items)
	}
}

// --- Metadata + catalog ------------------------------------------------

func TestE2E_GivenRegisteredEntity_WhenPostedToUserMetadata_ThenReturnsAbsolutePathAndName(t *testing.T) {
	app, _, _ := testApp(t)
	status, body := do(t, app, "user.metadata", nil)
	if status != http.StatusOK {
		t.Fatalf("status = %d body=%v", status, body)
	}
	data, ok := body["data"].(map[string]any)
	if !ok {
		t.Fatalf("no data envelope: %v", body)
	}
	if data["name"] != "user" {
		t.Fatalf("name = %v", data["name"])
	}
	// Path fields should be absolute — base URL concatenated with the action.
	if path, _ := data["path"].(string); path == "" || path[len(path)-len("user.list"):] != "user.list" {
		t.Fatalf("path = %q, want suffix user.list", path)
	}
}

func TestE2E_GivenMultipleRegisteredEntities_WhenPostedToAdminEntities_ThenCatalogListsEachOne(t *testing.T) {
	app, _, _ := testApp(t)
	_, body := do(t, app, "admin.entities", nil)
	data, ok := body["data"].(map[string]any)
	if !ok {
		t.Fatalf("no data: %v", body)
	}
	ents, ok := data["entities"].([]any)
	if !ok || len(ents) < 2 {
		t.Fatalf("entities = %v, want >= 2 entries", data["entities"])
	}
	// Assert that both user and session are present — other entities (like
	// Product) may also be here since the example keeps growing, but
	// user/session are the ones we've AutoMigrated.
	names := map[string]bool{}
	for _, e := range ents {
		m := e.(map[string]any)
		names[m["name"].(string)] = true
	}
	for _, want := range []string{"user", "session"} {
		if !names[want] {
			t.Errorf("catalog missing %q; saw %v", want, names)
		}
	}
}

// --- Custom actions ----------------------------------------------------
//
// The committed example doesn't declare any custom actions, so we can't
// exercise OnAction through it. Instead we exercise the runtime semantics:
// OnAction on an undeclared name should panic. If/when the example model
// adds //admin:action directives, this test can be strengthened to drive a
// wired handler.

func TestE2E_GivenUndeclaredActionName_WhenOnActionCalled_ThenPanics(t *testing.T) {
	_, a, _ := testApp(t)
	defer func() {
		if r := recover(); r == nil {
			t.Fatal("expected panic, got none")
		}
	}()
	a.User.OnAction("no-such-action", func(c *fiber.Ctx) error { return nil })
}

// --- ID generator hook -------------------------------------------------

func TestE2E_GivenCustomIDGenerator_WhenCreateCalledWithoutID_ThenGeneratorProvidesTheID(t *testing.T) {
	app, a, _ := testApp(t)
	// Even though User relies on DB auto-increment by default, installing a
	// generator should take effect — the handler checks idGenerator != nil
	// before falling through to the DB.
	a.User.SetIDGenerator(func() uint { return 9999 })

	_, body := do(t, app, "user.create", map[string]any{"name": "Fixed", "email": "f@x", "age": 1})
	if id, _ := body["id"].(float64); int(id) != 9999 {
		t.Fatalf("id = %v, want 9999 (generator override)", body["id"])
	}
}

func TestE2E_GivenCustomIDGeneratorAndClientProvidedID_WhenCreateCalled_ThenClientIDWins(t *testing.T) {
	app, a, _ := testApp(t)
	a.User.SetIDGenerator(func() uint { return 1 })

	_, body := do(t, app, "user.create", map[string]any{"id": 42, "name": "ClientID", "email": "c@x", "age": 1})
	if id, _ := body["id"].(float64); int(id) != 42 {
		t.Fatalf("id = %v, want 42 (client-provided wins)", body["id"])
	}
}

// --- Error envelope stability -----------------------------------------

func TestE2E_GivenHandlerReturningError_WhenResponseDecoded_ThenEnvelopeCarriesCodeAndMessage(t *testing.T) {
	app, _, _ := testApp(t)
	_, body := do(t, app, "user.get", map[string]any{"id": 0})
	e, ok := body["error"].(map[string]any)
	if !ok {
		t.Fatalf("no error envelope: %v", body)
	}
	if _, ok := e["code"]; !ok {
		t.Errorf("envelope missing code: %v", e)
	}
	if _, ok := e["message"]; !ok {
		t.Errorf("envelope missing message: %v", e)
	}
}
