package admin

import (
	"reflect"
	"time"

	"github.com/gofiber/fiber/v2"
)

type Catalog struct {
	Entities []Metadata `json:"entities"`
}

func (a *Admin) buildCatalog(base string) Catalog {
	names := a.Entities()
	out := Catalog{Entities: make([]Metadata, 0, len(names))}
	a.mu.RLock()
	defer a.mu.RUnlock()
	for _, n := range names {
		if m, ok := a.entities[n]; ok {
			out.Entities = append(out.Entities, m.buildMetadata(base))
		}
	}
	return out
}

type Metadata struct {
	Name         string          `json:"name"`
	DisplayName  string          `json:"display_name,omitempty"`
	MetadataPath string          `json:"metadata_path"`
	Path         string          `json:"path"`
	PrimaryKey   string          `json:"primary_key"`
	Fields       []FieldMetadata `json:"fields"`
	Actions      ActionsMetadata `json:"actions"`
	ListConfig   ListConfig      `json:"list_config"`
}

type FieldMetadata struct {
	Name        string         `json:"name"`
	DisplayName string         `json:"display_name,omitempty"`
	Type        string         `json:"type"`
	Primary     bool           `json:"primary,omitempty"`
	Readonly    bool           `json:"readonly,omitempty"`
	Writeonly   bool           `json:"writeonly,omitempty"`
	Required    bool           `json:"required,omitempty"`
	Filterable  bool           `json:"filterable,omitempty"`
	Sortable    bool           `json:"sortable,omitempty"`
	Searchable  bool           `json:"searchable,omitempty"`
	Options     []string       `json:"options,omitempty"`
	Validation  map[string]any `json:"validation,omitempty"`
}

type ActionsMetadata struct {
	Standard []string       `json:"standard"`
	Custom   []CustomAction `json:"custom,omitempty"`
}

type CustomAction struct {
	Name        string            `json:"name"`
	DisplayName string            `json:"display_name,omitempty"`
	Input       map[string]string `json:"input,omitempty"`
	Destructive bool              `json:"destructive,omitempty"`
	Handler     fiber.Handler     `json:"-"`
}

type ListConfig struct {
	DefaultPageSize int      `json:"default_page_size"`
	MaxPageSize     int      `json:"max_page_size"`
	DefaultSort     []string `json:"default_sort,omitempty"`
}

var standardActions = []string{"list", "get", "create", "update", "delete", "metadata"}

var timeType = reflect.TypeOf(time.Time{})

func (m *entityMeta) buildMetadata(base string) Metadata {
	out := Metadata{
		Name:         m.name,
		DisplayName:  m.displayName,
		MetadataPath: base + m.name + ".metadata",
		Path:         base + m.name + ".list",
		PrimaryKey:   m.idField.jsonName,
		Actions: ActionsMetadata{
			Standard: standardActions,
		},
		ListConfig: m.effectiveListConfig(),
	}
	for _, f := range m.fields {
		out.Fields = append(out.Fields, FieldMetadata{
			Name:        f.jsonName,
			DisplayName: f.displayName,
			Type:        inferFieldType(f),
			Primary:     f.isID,
			Readonly:    f.readonly || f.isID,
			Writeonly:   f.writeonly,
			Required:    f.required,
			Filterable:  f.filterable,
			Sortable:    f.sortable,
			Searchable:  f.searchable,
			Options:     f.enumOptions,
			Validation:  f.validation,
		})
	}
	for _, a := range m.actions {
		out.Actions.Custom = append(out.Actions.Custom, CustomAction{
			Name:        a.Name,
			DisplayName: a.DisplayName,
			Input:       a.Input,
			Destructive: a.Destructive,
		})
	}
	return out
}

func (m *entityMeta) effectiveListConfig() ListConfig {
	c := m.listConfig
	if c.DefaultPageSize == 0 {
		c.DefaultPageSize = 20
	}
	if c.MaxPageSize == 0 {
		c.MaxPageSize = 100
	}
	return c
}

func inferFieldType(f *fieldInfo) string {
	if len(f.enumOptions) > 0 {
		return "enum"
	}
	if f.goType != nil && f.goType == timeType {
		return "datetime"
	}
	switch f.kind {
	case reflect.String:
		return "string"
	case reflect.Bool:
		return "boolean"
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		return "int"
	case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64:
		return "uint"
	case reflect.Float32, reflect.Float64:
		return "float"
	case reflect.Struct:
		if f.goType != nil && f.goType == timeType {
			return "datetime"
		}
		return "object"
	case reflect.Slice, reflect.Array:
		return "array"
	case reflect.Map:
		return "object"
	}
	return "string"
}
