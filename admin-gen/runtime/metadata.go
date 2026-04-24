package runtime

import (
	"strings"

	"github.com/gofiber/fiber/v2"
)

// Metadata is the wire shape returned by <entity>.metadata. It mirrors the
// runtime admin package so clients can treat both implementations
// interchangeably.
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

// FieldMetadata describes one column on the wire.
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

// ActionsMetadata lists the standard and custom actions an entity supports.
type ActionsMetadata struct {
	Standard []string       `json:"standard"`
	Custom   []CustomAction `json:"custom,omitempty"`
}

// CustomAction is the wire shape of an entity-specific action declared via
// //admin:action. Input schemas are not emitted yet; clients should treat
// an absent Input field as "no arguments" for forward compatibility.
type CustomAction struct {
	Name        string `json:"name"`
	DisplayName string `json:"display_name,omitempty"`
	Destructive bool   `json:"destructive,omitempty"`
}

// ListConfig describes pagination defaults for <entity>.list.
type ListConfig struct {
	DefaultPageSize int      `json:"default_page_size"`
	MaxPageSize     int      `json:"max_page_size"`
	DefaultSort     []string `json:"default_sort,omitempty"`
}

// Catalog is the wire shape returned by admin.entities.
type Catalog struct {
	Entities []Metadata `json:"entities"`
}

// RequestBase returns the fully-qualified URL prefix under which the admin
// routes are mounted — e.g. "http://host:8080/api/v1/admin/". Metadata paths
// are built by concatenating this with "<entity>.<action>" so clients see
// absolute URLs regardless of how the router was grouped.
func RequestBase(c *fiber.Ctx) string {
	p := c.Path()
	if i := strings.LastIndex(p, "/"); i >= 0 {
		p = p[:i+1]
	}
	return c.BaseURL() + p
}
