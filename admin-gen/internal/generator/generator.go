// Package generator renders a schema.Entity slice into Go source files using
// the templates embedded from admin-gen/templates. Each generated file is
// passed through go/format so the output is always canonically formatted,
// whether or not the user runs `gofmt` afterward.
package generator

import (
	"bytes"
	"embed"
	"fmt"
	"go/format"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"text/template"

	"github.com/hokkung/go-admin/admin-gen/internal/schema"
)

//go:embed templates/*.tmpl
var templatesFS embed.FS

// Options configures a generator run.
type Options struct {
	// OutDir is where generated .go files land, e.g. "./internal/admin".
	OutDir string
	// OutPackage is the package name declared in generated files (typically
	// the last path component of OutDir).
	OutPackage string
	// ModelsImport is the import path of the package that declares the
	// annotated structs.
	ModelsImport string
	// RuntimeImport is the import path of admin-gen's runtime package the
	// generated code should import. Defaults to the canonical one if empty.
	RuntimeImport string
}

const defaultRuntimeImport = "github.com/hokkung/go-admin/admin-gen/runtime"

// Generate emits one handler file per entity plus a shared register.go.
// Existing files at those paths are overwritten — the generator assumes
// it owns its output directory.
func Generate(entities []schema.Entity, opts Options) error {
	if opts.RuntimeImport == "" {
		opts.RuntimeImport = defaultRuntimeImport
	}
	if err := os.MkdirAll(opts.OutDir, 0o755); err != nil {
		return fmt.Errorf("mkdir out: %w", err)
	}

	handlerTmpl, err := loadTemplate("handler.go.tmpl")
	if err != nil {
		return err
	}
	registerTmpl, err := loadTemplate("register.go.tmpl")
	if err != nil {
		return err
	}

	for _, e := range entities {
		data := buildHandlerData(e, opts)
		path := filepath.Join(opts.OutDir, strings.ToLower(e.GoName)+"_handlers.go")
		if err := renderTo(path, handlerTmpl, data); err != nil {
			return fmt.Errorf("%s: %w", e.GoName, err)
		}
	}

	regData := map[string]any{
		"OutPackage":    opts.OutPackage,
		"Entities":      entities,
		"RuntimeImport": opts.RuntimeImport,
	}
	if err := renderTo(filepath.Join(opts.OutDir, "register.go"), registerTmpl, regData); err != nil {
		return fmt.Errorf("register.go: %w", err)
	}
	return nil
}

func loadTemplate(name string) (*template.Template, error) {
	body, err := templatesFS.ReadFile("templates/" + name)
	if err != nil {
		return nil, fmt.Errorf("read template %s: %w", name, err)
	}
	return template.New(name).Parse(string(body))
}

type handlerData struct {
	schema.Entity
	OutPackage       string
	PackageName      string
	ModelsImport     string
	RuntimeImport    string
	NeedsUUID        bool
	UnexportedGoName string
	IDGoType         string
	IDField          schema.Field
	Filterable       []schema.Field
	Sortable         []schema.Field
	// Pre-joined "a", "b", "c" string for a single-case switch statement.
	// Empty when there are no filterable/sortable fields.
	FilterableCase string
	SortableCase   string
	// Go source for the `[]runtime.FieldMetadata{ ... }` body used by the
	// generated <entity>MetadataBase var. Pre-rendered so the template can
	// drop it in verbatim.
	FieldMetadataLiteral string
	// Body of the generated validate<Entity>(e *Entity) error function —
	// enum checks + per-field Validator attempts + entity-level Validator.
	ValidatorBody string
	// Effective pagination config, with framework defaults applied when the
	// entity didn't override them. Always non-zero so the template can emit
	// literals without conditionals.
	EffectiveDefaultPageSize int
	EffectiveMaxPageSize     int
	// Go literal of `[]runtime.SortSpec{...}` — empty if no default sort.
	DefaultSortLiteral string
	// Custom-action metadata, already filtered for non-empty names.
	Actions []schema.CustomAction
	// True when the constructor should seed h.idGenerator with uuid.New —
	// i.e. IDGen == "uuid". Non-UUID entities leave idGenerator nil and
	// rely on DB auto-increment unless the caller supplies their own.
	SeedUUIDIDGenerator bool
	// Go source for the `[]runtime.CustomAction{...}` literal embedded in
	// the static metadata var. Empty string when there are no actions.
	CustomActionsLiteral string
}

const (
	frameworkDefaultPageSize = 20
	frameworkMaxPageSize     = 100
)

func buildHandlerData(e schema.Entity, opts Options) handlerData {
	idField := *e.IDField()
	filterable := e.Filterable()
	sortable := e.Sortable()
	defaultPageSize := e.DefaultPageSize
	if defaultPageSize == 0 {
		defaultPageSize = frameworkDefaultPageSize
	}
	maxPageSize := e.MaxPageSize
	if maxPageSize == 0 {
		maxPageSize = frameworkMaxPageSize
	}
	return handlerData{
		Entity:                   e,
		OutPackage:               opts.OutPackage,
		PackageName:              e.PackageName,
		ModelsImport:             opts.ModelsImport,
		RuntimeImport:            opts.RuntimeImport,
		NeedsUUID:                idField.GoType == "uuid.UUID" || e.IDGen == "uuid",
		UnexportedGoName:         firstLower(e.GoName),
		IDGoType:                 idField.GoType,
		IDField:                  idField,
		Filterable:               filterable,
		Sortable:                 sortable,
		FilterableCase:           joinJSONNames(filterable),
		SortableCase:             joinJSONNames(sortable),
		FieldMetadataLiteral:     renderFieldMetadata(e.Fields),
		ValidatorBody:            renderValidatorBody(e),
		EffectiveDefaultPageSize: defaultPageSize,
		EffectiveMaxPageSize:     maxPageSize,
		DefaultSortLiteral:       renderDefaultSort(e.DefaultSort),
		Actions:                  e.CustomActions,
		SeedUUIDIDGenerator:      e.IDGen == "uuid",
		CustomActionsLiteral:     renderCustomActions(e.CustomActions),
	}
}

// renderCustomActions emits the `[]runtime.CustomAction{...}` literal body
// for the metadata var. Returns empty so the template can omit the Custom
// field entirely when there are no actions.
func renderCustomActions(actions []schema.CustomAction) string {
	if len(actions) == 0 {
		return ""
	}
	var b strings.Builder
	for _, a := range actions {
		b.WriteString("\t\t{Name: ")
		b.WriteString(strconv.Quote(a.Name))
		if a.DisplayName != "" {
			b.WriteString(", DisplayName: ")
			b.WriteString(strconv.Quote(a.DisplayName))
		}
		if a.Destructive {
			b.WriteString(", Destructive: true")
		}
		b.WriteString("},\n")
	}
	return b.String()
}

// renderValidatorBody emits the body of a generated `validate<Entity>(e *X)`
// function. The order mirrors the runtime admin package's runValidators:
// enum checks first (so clients see the most specific error), then every
// field's Validator, then the entity's Validator. Errors carry the JSON
// field name as a prefix, again matching the runtime version.
func renderValidatorBody(e schema.Entity) string {
	var b strings.Builder
	// Enum checks. `string(e.Field)` so the call accepts both `string` and
	// user-defined string aliases (e.g. `type Status string`). If the
	// underlying kind isn't string-compatible the generated code fails to
	// compile, which is a clearer signal than a silent skip.
	for _, f := range e.Fields {
		if len(f.EnumOptions) == 0 {
			continue
		}
		b.WriteString("\tif err := runtime.EnforceEnum(")
		b.WriteString(strconv.Quote(f.JSONName))
		b.WriteString(", string(e.")
		b.WriteString(f.GoName)
		b.WriteString("), []string{")
		for i, opt := range f.EnumOptions {
			if i > 0 {
				b.WriteString(", ")
			}
			b.WriteString(strconv.Quote(opt))
		}
		b.WriteString("}); err != nil {\n\t\treturn err\n\t}\n")
	}

	// Field-level Validator attempts. runtime.CallFieldValidator tries the
	// value form first and the pointer form second, returning the error
	// with the JSON field name as a prefix — matches the runtime admin
	// package's runValidators loop.
	for _, f := range e.Fields {
		b.WriteString("\tif err := runtime.CallFieldValidator(")
		b.WriteString(strconv.Quote(f.JSONName))
		b.WriteString(", e.")
		b.WriteString(f.GoName)
		b.WriteString(", &e.")
		b.WriteString(f.GoName)
		b.WriteString("); err != nil {\n\t\treturn err\n\t}\n")
	}
	// Entity-level Validator — no prefix, matching runtime/admin.
	b.WriteString("\tif err := runtime.CallEntityValidator(*e, e); err != nil {\n\t\treturn err\n\t}\n")
	b.WriteString("\treturn nil\n")
	return b.String()
}

func renderDefaultSort(specs []schema.SortSpec) string {
	if len(specs) == 0 {
		return ""
	}
	var b strings.Builder
	b.WriteString("[]runtime.SortSpec{")
	for i, s := range specs {
		if i > 0 {
			b.WriteString(", ")
		}
		b.WriteString("{Field: ")
		b.WriteString(strconv.Quote(s.Field))
		b.WriteString(", Order: ")
		b.WriteString(strconv.Quote(s.Order))
		b.WriteString("}")
	}
	b.WriteString("}")
	return b.String()
}

// renderFieldMetadata emits the body of a `[]runtime.FieldMetadata{...}` slice
// literal — one element per field, commas/newlines included. The caller drops
// it inside a slice literal in the generated template. `go/format` cleans up
// the indentation afterward.
func renderFieldMetadata(fields []schema.Field) string {
	var b strings.Builder
	for _, f := range fields {
		b.WriteString("{Name: ")
		b.WriteString(strconv.Quote(f.JSONName))
		if f.DisplayName != "" {
			b.WriteString(", DisplayName: ")
			b.WriteString(strconv.Quote(f.DisplayName))
		}
		b.WriteString(", Type: ")
		b.WriteString(strconv.Quote(inferWireType(f)))
		// Primary implies Readonly in the admin package output; match that so
		// client code does not need a special case for the id field.
		if f.IsID {
			b.WriteString(", Primary: true, Readonly: true")
		} else if f.Readonly {
			b.WriteString(", Readonly: true")
		}
		if f.Writeonly {
			b.WriteString(", Writeonly: true")
		}
		if f.Required {
			b.WriteString(", Required: true")
		}
		if f.Filterable {
			b.WriteString(", Filterable: true")
		}
		if f.Sortable {
			b.WriteString(", Sortable: true")
		}
		if f.Searchable {
			b.WriteString(", Searchable: true")
		}
		if len(f.EnumOptions) > 0 {
			b.WriteString(", Options: []string{")
			for i, opt := range f.EnumOptions {
				if i > 0 {
					b.WriteString(", ")
				}
				b.WriteString(strconv.Quote(opt))
			}
			b.WriteString("}")
		}
		if len(f.Validation) > 0 {
			b.WriteString(", Validation: map[string]any{")
			keys := make([]string, 0, len(f.Validation))
			for k := range f.Validation {
				keys = append(keys, k)
			}
			sort.Strings(keys)
			for i, k := range keys {
				if i > 0 {
					b.WriteString(", ")
				}
				b.WriteString(strconv.Quote(k))
				b.WriteString(": ")
				b.WriteString(strconv.Quote(f.Validation[k]))
			}
			b.WriteString("}")
		}
		b.WriteString("},\n")
	}
	return b.String()
}

// inferWireType maps a source-level Go type expression onto the coarse type
// vocabulary the metadata endpoint exposes ("string", "int", "datetime", …).
// The admin package does this via reflect.Kind at runtime; here we only have
// the type string from the AST, so a few well-known names are special-cased
// and anything else falls back to "object". The enum tag wins outright.
func inferWireType(f schema.Field) string {
	if len(f.EnumOptions) > 0 {
		return "enum"
	}
	t := strings.TrimPrefix(f.GoType, "*")
	switch t {
	case "time.Time":
		return "datetime"
	case "uuid.UUID":
		return "string"
	case "string":
		return "string"
	case "bool":
		return "boolean"
	case "int", "int8", "int16", "int32", "int64":
		return "int"
	case "uint", "uint8", "uint16", "uint32", "uint64":
		return "uint"
	case "float32", "float64":
		return "float"
	case "json.RawMessage":
		return "object"
	}
	if strings.HasPrefix(t, "[]") || (len(t) > 0 && t[0] == '[') {
		return "array"
	}
	if strings.HasPrefix(t, "map[") {
		return "object"
	}
	return "object"
}

func joinJSONNames(fields []schema.Field) string {
	if len(fields) == 0 {
		return ""
	}
	parts := make([]string, len(fields))
	for i, f := range fields {
		parts[i] = `"` + f.JSONName + `"`
	}
	return strings.Join(parts, ", ")
}

func firstLower(s string) string {
	if s == "" {
		return s
	}
	return strings.ToLower(s[:1]) + s[1:]
}

func renderTo(path string, tmpl *template.Template, data any) error {
	var buf bytes.Buffer
	if err := tmpl.Execute(&buf, data); err != nil {
		return fmt.Errorf("execute template: %w", err)
	}
	formatted, err := format.Source(buf.Bytes())
	if err != nil {
		// On format failure, write the unformatted source anyway so the user
		// can inspect what went wrong rather than a truncated file.
		_ = os.WriteFile(path, buf.Bytes(), 0o644)
		return fmt.Errorf("format %s: %w", path, err)
	}
	return os.WriteFile(path, formatted, 0o644)
}
