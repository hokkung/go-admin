# admin-gen

Code-generating alternative to the runtime [`admin/`](../admin/) library. You annotate your Go structs, run a CLI, and get plain, readable Go files — handlers, stores, and routing wiring — that your team owns from that moment on.

```
type User struct { … }        →   admin-gen   →   user_handlers.go
                                                  user_store_gorm.go
                                                  register.go
```

No reflection at request time, no magic dispatch. The generated code is roughly what you'd write by hand, so filters/sort allowlists are explicit `switch` statements you can read, grep, and modify.

---

## Why this over the runtime `admin/` package

The runtime version does everything through reflection at request time. That's fine for small projects but trades away:

- **Readability** — the code that serves `POST /user.create` lives across several `reflect.Value` hops. You can't step through a request in the debugger and see what your app is doing.
- **Performance** — every request pays a reflection tax.
- **Ownership** — you can't easily override one entity's update handler without forking the framework.

`admin-gen` trades a little discipline (run the generator when the schema changes) for plain Go you can own.

---

## Workflow

1. Write your models, annotate the ones you want admin routes for:

   ```go
   package models

   //admin:generate
   type User struct {
       ID    uint   `json:"id" admin:"id,sortable"`
       Name  string `json:"name" admin:"filterable,sortable,required"`
       Email string `json:"email" admin:"filterable,required"`
   }
   ```

2. Run the generator:

   ```bash
   admin-gen -in ./models -out ./admin
   ```

3. Open the generated files, skim them, commit them. They live in your repo; they're your code.

4. Wire the generated `admin.Register(app, db)` call into your server:

   ```go
   a := admin.Register(db)
   app := fiber.New()
   a.Mount(app.Group("/admin"))
   ```

When the schema changes, re-run the generator. Committing the output means schema drift shows up in PR diffs.

---

## What's in scope for this first cut

Built in:
- CRUD: `create`, `get`, `update` (partial merge), `delete`, `list`.
- List: filter (`eq` / `ne` / `lt` / `lte` / `gt` / `gte` / `contains` / `in`), sort, pagination — all pushed into SQL via GORM.
- Per-entity `default_page_size` / `max_page_size` / `default_sort` via directive args; reflected in both the list handler and the `<entity>.metadata` payload.
- ID generation: DB auto-increment (default), in-handler `uuid.New` (for UUID fields), or a caller-supplied `func() <IDType>` via `<Handler>.SetIDGenerator(…)`.
- Error envelope matching [CLAUDE.md](../CLAUDE.md): `{"error":{"code":"…","message":"…"}}`.
- Minimal [runtime package](runtime/) that generated code imports — just shared JSON shapes and a few helpers.
- Validation on create/update in the order the runtime admin package uses: `enum` tag allowlists (server-side enforced), each field's `Validate() error` (both value and pointer receiver, JSON field name prepended to the error), then the entity's `Validate() error`.
- Metadata (`<entity>.metadata`) and catalog (`admin.entities`) endpoints — the shape is computed at generation time and emitted as a static var, so serving metadata costs a map copy and no reflection.
- Same-package embedded anonymous struct fields are flattened into the generated handlers (filter/sort allowlists, metadata, validator).
- Custom actions declared via `//admin:action name=… display=… destructive`; the generator emits the route, metadata entry, and a `<Handler>.OnAction(name, handler)` setter. Handler bodies live in your code, not the generated file.

Deferred (trivially templatable, happy to add next iteration):
- Custom-action input schemas (`input={"reason":"string"}`) — the wire slot exists, the directive parser doesn't yet.
- Cross-package embed resolution (needs go/types).
- Column-name override for when `json:"name"` differs from the DB column.

---

## Layout

```
admin-gen/
├── cmd/admin-gen/          the CLI entry point (go install here)
├── internal/
│   ├── schema/             entity / field intermediate representation
│   ├── parser/             go/ast walker; extracts annotations + tags
│   └── generator/          template driver + go/format pass
└── runtime/                small shared package imported by generated code
```

A runnable example that exercises the whole flow lives at
[../example/cmd/admin-gen/](../example/cmd/admin-gen/):

```
example/cmd/admin-gen/
├── models/                 annotated source models
├── admin/                  pre-generated output (committed so you can read it)
└── main.go                 wires fiber + gorm + postgres
```

---

## The generated code's shape

Each annotated struct produces three files plus one shared `register.go`:

- `<entity>_handlers.go` — the five HTTP handlers (create/get/update/delete/list), each ~20-40 lines of straightforward Go. Filter/sort allowlists are explicit.
- `<entity>_store_gorm.go` — the GORM calls. Isolated so you can swap backends (write a Postgres-native one, add a cache) without touching the handlers.
- `register.go` — a single `Register(db *gorm.DB) *Admin` that a `main.go` calls, plus `(*Admin).Mount(fiber.Router)`.

A sample handler (from `example/admin/user_handlers.go`):

```go
func (h *UserHandler) list(c *fiber.Ctx) error {
    var q runtime.ListQuery
    if err := decodeQuery(c, &q); err != nil {
        return runtime.WriteError(c, err)
    }
    tx := h.db.WithContext(c.Context()).Model(&models.User{})
    for _, f := range q.Filters {
        switch f.Field {
        case "name", "email":                // only filterable fields listed
            tx = runtime.ApplyGormFilter(tx, f.Field, f)
        default:
            return runtime.WriteError(c, runtime.BadRequest("not filterable: "+f.Field))
        }
    }
    …
}
```

This is what we mean by "normal code": no magic, readable diff in PRs, debug-steppable.

---

## Contract for `//admin:generate`

The directive goes on the line **immediately above** a struct declaration:

```go
//admin:generate
type User struct { … }
```

Optional args (space-separated `key=value`; quote values that contain commas or spaces):

| Key                 | Meaning                                                                 | Default                                 |
| ------------------- | ----------------------------------------------------------------------- | --------------------------------------- |
| `name`              | Wire-facing entity name                                                 | lowercased struct name (`user`)         |
| `display`           | Human label (surfaced on `<entity>.metadata`)                           | struct name                             |
| `idgen`             | `increment` → let the DB auto-assign; `uuid` → generate in-handler      | `increment` unless id is `uuid.UUID`    |
| `default_page_size` | Default `page_size` when the request omits it                           | `20`                                    |
| `max_page_size`     | Upper bound on `page_size`; larger requests are clamped                 | `100`                                   |
| `default_sort`      | Sort clause applied when the request has no `sort` — `"field:desc,…"`   | none                                    |

Field-level behavior uses the same struct tags as the runtime library: `admin:"id,filterable,sortable,required,readonly,writeonly,searchable"`, `json:"…"`, `enum:"…"`, `validate:"…"`.

Embedded anonymous struct fields declared in the **same package** are flattened into the generated handlers — useful for shared mixins like `type Timestamps struct { CreatedAt, UpdatedAt time.Time }`. Cross-package embeds (e.g. `gorm.Model`) aren't resolved yet and are silently skipped; copy their fields inline if you need them.

### Custom actions

Declare per-entity actions with one or more `//admin:action` comments right above the struct:

```go
//admin:generate
//admin:action name=activate display="Activate User"
//admin:action name=purge destructive
type User struct { … }
```

Action names must not collide with the standard actions (`create`, `get`, `update`, `delete`, `list`, `metadata`) — the generator rejects duplicates at parse time.

The generator emits a `POST /<entity>.<action>` route, a metadata entry, and a setter on the handler. Wire the actual handler at startup:

```go
a := admin.Register(db)
a.User.OnAction("activate", func(c *fiber.Ctx) error {
    // your business logic
    return c.JSON(fiber.Map{"ok": true})
})
app := fiber.New()
a.Mount(app.Group("/admin"))
```

Unwired action calls return `501 NOT_IMPLEMENTED` with the action name; `OnAction` panics on unknown names so typos fail at startup.

### ID generators

Each generated handler exposes `SetIDGenerator(func() <IDType>)`. Call it after `Register` to override the built-in behavior (DB auto-increment for integer ids, `uuid.New` for UUID ids). The generator runs only when the client sent a zero-valued id, so client-provided ids still win.
