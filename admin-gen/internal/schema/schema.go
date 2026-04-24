// Package schema is the intermediate representation the parser hands to the
// generator. Keep it dumb — no behavior beyond trivial accessors — so it's
// easy to snapshot for tests and to serialize if we ever want a JSON schema
// input mode alongside annotated Go.
package schema

// Entity is one admin-managed struct.
type Entity struct {
	// Go-level identity.
	GoName      string // struct name, e.g. "User"
	PackagePath string // import path of the source package, e.g. "example.com/app/models"
	PackageName string // last segment of PackagePath, e.g. "models"

	// Wire-facing identity.
	Name        string // e.g. "user" (lowercased GoName by default)
	DisplayName string // e.g. "Users" (GoName if unset)

	Fields []Field

	// IDGen is "", "increment", or "uuid". Empty means "let whatever the
	// storage default is take over" — for GORM that means DB auto-increment.
	IDGen string

	// ListConfig overrides for this entity. Zero values mean "use the
	// framework default" — currently 20 / 100 in the generated handlers.
	DefaultPageSize int
	MaxPageSize     int
	// DefaultSort is applied when the incoming request has no Sort clause.
	// Every Field referenced here must also be Sortable on the entity;
	// the generator does not validate this, the compiler/test round does.
	DefaultSort []SortSpec

	// CustomActions declared via //admin:action directives. The generator
	// emits a metadata entry, a route, and a runtime-registered dispatch
	// slot for each; the actual handler is supplied by the caller at
	// startup via <Handler>.OnAction(name, handler).
	CustomActions []CustomAction
}

// CustomAction mirrors the runtime admin package's CustomAction. Input
// schemas are intentionally omitted for now — add them when a caller
// actually needs them rather than speculatively.
type CustomAction struct {
	Name        string
	DisplayName string
	Destructive bool
}

// SortSpec mirrors runtime.SortSpec. Duplicated here so the schema package
// stays free of runtime imports (the parser feeds the generator, which
// owns runtime coupling).
type SortSpec struct {
	Field string
	Order string
}

// IDField returns the field marked as the primary key, or nil if none.
func (e *Entity) IDField() *Field {
	for i := range e.Fields {
		if e.Fields[i].IsID {
			return &e.Fields[i]
		}
	}
	return nil
}

// Filterable / Sortable returns only the fields with the respective flag —
// templates consume these for the handler switch statements.
func (e *Entity) Filterable() []Field {
	var out []Field
	for _, f := range e.Fields {
		if f.Filterable {
			out = append(out, f)
		}
	}
	return out
}

func (e *Entity) Sortable() []Field {
	var out []Field
	for _, f := range e.Fields {
		if f.Sortable {
			out = append(out, f)
		}
	}
	return out
}

// Field is one column/property on an Entity.
type Field struct {
	// Go-level identity.
	GoName string // struct field name, e.g. "Email"
	GoType string // source-level type expression, e.g. "string", "uuid.UUID", "time.Time"

	// Wire-facing identity.
	JSONName    string // encoding/json name (from the json:"…" tag)
	DisplayName string // from display:"…"

	// Flags from admin:"…".
	IsID       bool
	Filterable bool
	Sortable   bool
	Searchable bool
	Readonly   bool
	Writeonly  bool
	Required   bool

	// Extras from other tags.
	EnumOptions []string          // from enum:"a,b,c"
	Validation  map[string]string // from validate:"format=email,..."
}
