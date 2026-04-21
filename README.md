# go-admin

A plug-and-play Go library that turns any struct into a full admin CRUD API. Register a model, mount it onto a Fiber app, and you get create / get / update / delete / list with filtering, sorting, pagination, and a self-describing metadata endpoint for generic UIs.

```go
a := admin.New()
a.MustRegister(&User{})
app := fiber.New()
a.Mount(app)
app.Listen(":8080")
```

---

## Features

- **Reflection-based registration** — pass any struct, pay no boilerplate. Fields are introspected via `json` and `admin` struct tags.
- **One wire convention** — every endpoint is `POST /<entity>.<action>`, matching [CLAUDE.md](CLAUDE.md).
- **Standard actions** — `create`, `get`, `update`, `delete`, `list`, `metadata`.
- **List querying** — filter operators (`eq`, `ne`, `lt`, `lte`, `gt`, `gte`, `contains`, `in`), multi-field sort, pagination.
- **Partial update** — update performs read-merge-write, so callers send only the fields that change.
- **Pluggable storage** — in-memory by default; GORM adapter included. Anything satisfying the `Storage` interface drops in.
- **Auto ID generation** — opt in with `admin.WithIDGenerator(admin.AutoUUID())` or `admin.AutoIncrement()`; zero-valued ids are filled in before the row is persisted, and UUIDs round-trip through `get`/`update`/`delete` via the field's own `json.Unmarshaler`.
- **Validation** — implement `admin.Validator` (`Validate() error`) on any entity or field type to plug in custom rules, and `enum` tags are enforced server-side automatically. Failures return `{"error":{"code":"VALIDATION_FAILED","message":"<field>: <reason>"}}`.
- **Metadata endpoint** — `POST /<entity>.metadata` returns a JSON schema describing fields, validation, enum options, actions, and list defaults. Designed so a generic frontend can render forms and tables without hard-coding per entity.
- **Catalog endpoint** — `POST /admin.entities` returns the full metadata for every registered entity in one call, so a UI can bootstrap its whole nav/sidebar with a single request.
- **Custom actions** — register additional entity-scoped actions (e.g. `user.resetPassword`) that show up in metadata and are dispatched through the same router.
- **Uniform error envelope** — every error response is `{"error":{"code":"…","message":"…"}}`.

---

## Install

```bash
go get github.com/hokkung/go-admin
```

Minimum Go: 1.25.

---

## Quick start

```go
package main

import (
    "github.com/gofiber/fiber/v2"
    "github.com/hokkung/go-admin/admin"
)

type User struct {
    ID    uint   `json:"id" admin:"id,sortable"`
    Name  string `json:"name" admin:"required,searchable,filterable,sortable"`
    Email string `json:"email" admin:"required,filterable"`
    Age   int    `json:"age" admin:"filterable,sortable"`
}

func main() {
    a := admin.New()
    a.MustRegister(&User{})

    app := fiber.New()
    a.Mount(app)
    app.Listen(":8080")
}
```

```bash
curl -X POST localhost:8080/user.create -d '{"name":"Alice","email":"a@x.com","age":30}'
curl -X POST localhost:8080/user.list   -d '{"filters":[{"field":"age","op":"gt","value":25}],"sort":[{"field":"age","order":"desc"}]}'
curl -X POST localhost:8080/user.metadata
```

A fuller example with enums, validation, list config, and custom actions is in [example/cmd/main.go](example/cmd/main.go). A GORM-backed example is in [example/gorm/main.go](example/gorm/main.go).

---

## Struct tags

### `admin:"..."` — comma-separated flags

| Flag          | Effect                                                                 |
| ------------- | ---------------------------------------------------------------------- |
| `id`          | Marks the primary key. Also inferred if the field is named `ID`.        |
| `filterable`  | Allows this field to appear in `list` filters.                          |
| `sortable`    | Allows this field to appear in `list` sort specs.                       |
| `searchable`  | Hint for UI free-text search (metadata only; no server-side enforcement).|
| `readonly`    | Metadata hint — UI should not offer this field in create/update forms.  |
| `writeonly`   | Metadata hint — field is accepted on input but should not be rendered.   |
| `required`    | Metadata hint for the UI. Server does not enforce presence.             |

### `enum:"a,b,c"`
Declares allowed values. Surfaces as `options` in metadata and flips `type` to `"enum"`.

### `validate:"format=email,minLength=2,maxLength=255"`
Arbitrary key=value pairs. Numeric values are parsed as ints. Emitted verbatim in metadata for the UI to enforce.

### `display:"Human Label"`
Field-level display name shown in metadata.

### `json:"…"`
Standard encoding/json tag — determines the wire name used by every endpoint.

Example:
```go
type User struct {
    ID        uint      `json:"id"         admin:"id,sortable"`
    Email     string    `json:"email"      admin:"required,searchable,filterable,sortable" validate:"format=email,maxLength=255"`
    Role      string    `json:"role"       admin:"filterable" enum:"user,admin,super"`
    Password  string    `json:"password"   admin:"writeonly,required"`
    CreatedAt time.Time `json:"created_at" admin:"readonly,sortable,filterable"`
}
```

---

## Registration options

```go
a.MustRegister(&User{},
    admin.WithName("users"),                        // override entity name (default: lowercased struct name)
    admin.WithDisplayName("Users"),                 // human label for the UI
    admin.WithStorage(gormstore.New(db)),           // custom storage; default is in-memory
    admin.WithIDGenerator(admin.AutoUUID()),        // or admin.AutoIncrement(); optional
    admin.WithListConfig(admin.ListConfig{
        DefaultPageSize: 20,
        MaxPageSize:     100,
        DefaultSort:     []string{"-created_at"},
    }),
    admin.WithAction(admin.CustomAction{
        Name:        "resetPassword",
        DisplayName: "Reset Password",
        Input:       map[string]string{"userId": "uint"},
        Handler: func(c *fiber.Ctx) error {
            return c.JSON(fiber.Map{"ok": true})
        },
    }),
)
```

### ID generation

| Helper                  | Fills                                              | Use when                                                         |
| ----------------------- | -------------------------------------------------- | ---------------------------------------------------------------- |
| `admin.AutoUUID()`      | `uuid.UUID` (or a type convertible from it)        | Primary key is a UUID and you want the server to mint it         |
| `admin.AutoIncrement()` | `int*` / `uint*` / `string` (`<name>_<n>`)         | In-memory / test setups where you want a stable monotonic id     |
| *(not set)*             | nothing — value is forwarded as-is                 | Let the database assign the id (SQL auto-increment, `gen_random_uuid()`, etc.) |

Generators only run when the id field is the zero value, so callers can still supply their own id on create and have it respected.

---

## Validation

Before every `create` and `update`, the framework runs validators against the incoming entity. Two mechanisms:

### 1. `Validator` interface — for custom rules

Any type that implements `Validate() error` gets called. This works for entities and for individual field types.

```go
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
```

Now any `Product.Status` value outside the allowed set is rejected:

```
POST /product.create  {"status":"pending", ...}
→ 400 {"error":{"code":"VALIDATION_FAILED","message":"status: invalid status \"pending\" (allowed: active, inactive)"}}
```

You can also attach `Validate()` to the entity struct itself to run cross-field checks (e.g., `Price >= 0`).

### 2. `enum` tag — for simple allowed-set checks

Declaring allowed values in the tag is equivalent to writing a validator by hand for the common case:

```go
type User struct {
    Role string `json:"role" admin:"filterable" enum:"user,admin,super"`
}
```

The tag also drives the UI metadata (`type: "enum"`, `options: [...]`), so a single declaration covers both UI hint and server-side enforcement. An empty string is treated as unset and allowed through — pair with `admin:"required"` if the field is mandatory.

Use the `Validator` interface when the rule is more than just a set of allowed strings; use `enum` when it isn't.

---

## API reference

All requests are `POST` with a JSON body. All responses are JSON.

### `POST /<entity>.create`
Request: full entity body.
```json
{"name":"Alice","email":"a@x.com","age":30}
```
Response: the created entity (id auto-assigned if zero).

### `POST /<entity>.get`
Request:
```json
{"id": 1}
```
Response: the entity, or `404 NOT_FOUND`.

### `POST /<entity>.update`
Request: id plus any subset of fields. The server reads the existing row, overlays the provided fields, and writes it back.
```json
{"id": 1, "age": 31}
```
Response: the updated entity.

### `POST /<entity>.delete`
Request:
```json
{"id": 1}
```
Response:
```json
{"deleted": true, "id": 1}
```

### `POST /<entity>.list`
Request (every field optional):
```json
{
  "filters":  [{"field":"age","op":"gt","value":25}],
  "sort":     [{"field":"age","order":"desc"}],
  "page":     1,
  "page_size": 20
}
```
Response:
```json
{
  "items":     [...],
  "total":     42,
  "page":      1,
  "page_size": 20
}
```

Filter operators: `eq` (default), `ne`, `lt`, `lte`, `gt`, `gte`, `contains` (string fields), `in` (value must be an array).

Only fields tagged `filterable` / `sortable` are accepted; anything else is rejected with `400 BAD_REQUEST`.

### `POST /<entity>.metadata`
Returns the schema used for UI autogeneration.

```json
{
  "data": {
    "name": "users",
    "displayName": "Users",
    "primaryKey": "id",
    "fields": [
      {"name":"id","type":"uint","primary":true,"readonly":true,"sortable":true},
      {"name":"email","type":"string","required":true,"filterable":true,"sortable":true,"searchable":true,
       "validation":{"format":"email","maxLength":255}},
      {"name":"role","type":"enum","filterable":true,"options":["user","admin","super"]},
      {"name":"password","type":"string","writeonly":true,"required":true},
      {"name":"created_at","type":"datetime","readonly":true,"sortable":true,"filterable":true}
    ],
    "actions": {
      "standard": ["list","get","create","update","delete","metadata"],
      "custom": [
        {"name":"resetPassword","displayName":"Reset Password","input":{"userId":"uint"}},
        {"name":"suspend","displayName":"Suspend User","input":{"userId":"uint","reason":"string"},"destructive":true}
      ]
    },
    "listConfig": {"defaultPageSize":20,"maxPageSize":100,"defaultSort":["-created_at"]}
  }
}
```

Type mapping: `string`, `int`, `uint`, `float`, `boolean`, `datetime` (for `time.Time`), `enum` (when `enum` tag is set), `array`, `object`.

### `POST /<entity>.<customAction>`
Invokes a handler registered via `WithAction`. Request/response shapes are whatever the handler defines.

### `POST /admin.entities`
Framework-level endpoint (no body). Returns the full metadata for every registered entity — useful for bootstrapping a generic admin UI with a single request.

```json
{
  "data": {
    "entities": [
      { "name": "product", "path": "http://host:8080/admin/product.metadata", "primaryKey": "id", "fields": [...], "actions": {...}, "listConfig": {...} },
      { "name": "users",   "path": "http://host:8080/admin/users.metadata",   "primaryKey": "id", "fields": [...], "actions": {...}, "listConfig": {...} }
    ]
  }
}
```

Each entry has the same shape as `<entity>.metadata`. The `path` field is a fully-qualified URL derived from the incoming request — it composes correctly whether admin is mounted at the root or under a group (e.g. `a.Mount(app.Group("/admin"))`). The entity name `admin` is reserved for this namespace — use `admin.WithName("…")` if your model legitimately needs that name.

---

## Error format

Every failure response follows this shape — see [CLAUDE.md](CLAUDE.md):

```json
{
  "error": {
    "code": "NOT_FOUND",
    "message": "resource not found"
  }
}
```

Built-in codes: `NOT_FOUND`, `BAD_REQUEST`, `INVALID_ACTION`, `INVALID_ENTITY`, `ALREADY_EXISTS`, `METHOD_NOT_ALLOWED`, `INTERNAL`. Custom `Storage` implementations can return any `*admin.Error` and the framework preserves the status, code, and message.

---

## Storage

### In-memory (default)

```go
a.MustRegister(&User{})
```
Auto-generates IDs (string for `string` id fields, int for int/uint kinds). Good for demos and tests.

### GORM — [admin/gormstore](admin/gormstore/gormstore.go)

```go
import "github.com/hokkung/go-admin/admin/gormstore"

db, _ := gorm.Open(sqlite.Open("app.db"), &gorm.Config{})
db.AutoMigrate(&User{})

a.MustRegister(&User{}, admin.WithStorage(gormstore.New(db)))
```

Uses GORM's generic methods (`Create`, `First`, `Save`, `Delete`, `Find`). Not in a transitive import path unless you opt in, so the base admin package stays free of GORM.

**Caveat:** the current `Storage.List` contract returns all rows and filter/sort/pagination happen in memory. Fine for small tables; for large datasets you'll want to extend `Storage` so the query is pushed into SQL.

### Custom storage

Implement `admin.Storage`:

```go
type Storage interface {
    Create(ctx context.Context, meta StorageMeta, entity reflect.Value) (reflect.Value, error)
    Get   (ctx context.Context, meta StorageMeta, id any) (reflect.Value, error)
    Update(ctx context.Context, meta StorageMeta, id any, entity reflect.Value) (reflect.Value, error)
    Delete(ctx context.Context, meta StorageMeta, id any) error
    List  (ctx context.Context, meta StorageMeta) ([]reflect.Value, error)
}
```

`StorageMeta` exposes the entity name, struct type, and ID helpers so adapters can stay model-agnostic.

---

## Mounting

Admin routes mount onto any `fiber.Router` — pass the app directly or a route group:

```go
app := fiber.New()
app.Use(logger.New(), cors.New(), myAuth())

a := admin.New()
a.MustRegister(&User{})
a.Mount(app.Group("/admin"))   // routes become POST /admin/user.list, etc.

app.Listen(":8080")
```

This keeps admin decoupled from your middleware choices and lets non-admin endpoints coexist on the same app.

---

## Repo layout

```
admin/              core library
├── admin.go        registration, options, entity index
├── entity.go       reflection + tag parsing
├── errors.go       error envelope
├── handler.go      fiber routing + dispatch
├── metadata.go     schema types + builder
├── query.go        filter / sort / pagination
├── storage.go      Storage interface + in-memory impl
└── gormstore/      GORM adapter (optional import)
example/
├── cmd/            in-memory example with custom actions
└── gorm/           GORM + SQLite example
```
