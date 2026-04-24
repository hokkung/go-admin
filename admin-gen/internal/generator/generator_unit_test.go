package generator

import (
	"strings"
	"testing"

	"github.com/hokkung/go-admin/admin-gen/internal/schema"
)

// --- inferWireType ----------------------------------------------------

func TestInferWireType_GivenGoType_WhenResolved_ThenReturnsWireKind(t *testing.T) {
	cases := []struct {
		name   string
		field  schema.Field
		want   string
		reason string
	}{
		{"given enum options when resolved then returns enum regardless of Go type", schema.Field{GoType: "string", EnumOptions: []string{"a"}}, "enum", ""},
		{"given time.Time when resolved then returns datetime", schema.Field{GoType: "time.Time"}, "datetime", ""},
		{"given pointer to time.Time when resolved then returns datetime", schema.Field{GoType: "*time.Time"}, "datetime", ""},
		{"given uuid.UUID when resolved then returns string", schema.Field{GoType: "uuid.UUID"}, "string", ""},
		{"given plain string when resolved then returns string", schema.Field{GoType: "string"}, "string", ""},
		{"given bool when resolved then returns boolean", schema.Field{GoType: "bool"}, "boolean", ""},
		{"given int when resolved then returns int", schema.Field{GoType: "int"}, "int", ""},
		{"given int64 when resolved then returns int", schema.Field{GoType: "int64"}, "int", ""},
		{"given uint when resolved then returns uint", schema.Field{GoType: "uint"}, "uint", ""},
		{"given uint32 when resolved then returns uint", schema.Field{GoType: "uint32"}, "uint", ""},
		{"given float32 when resolved then returns float", schema.Field{GoType: "float32"}, "float", ""},
		{"given float64 when resolved then returns float", schema.Field{GoType: "float64"}, "float", ""},
		{"given slice when resolved then returns array", schema.Field{GoType: "[]string"}, "array", ""},
		{"given fixed-size array when resolved then returns array", schema.Field{GoType: "[16]byte"}, "array", ""},
		{"given map when resolved then returns object", schema.Field{GoType: "map[string]int"}, "object", ""},
		{"given json.RawMessage when resolved then returns object", schema.Field{GoType: "json.RawMessage"}, "object", ""},
		{"given unknown named type when resolved then returns object", schema.Field{GoType: "models.ProductStatus"}, "object", "no type resolution without go/types"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := inferWireType(tc.field); got != tc.want {
				t.Fatalf("got %q, want %q (%s)", got, tc.want, tc.reason)
			}
		})
	}
}

// --- renderFieldMetadata ----------------------------------------------

func TestRenderFieldMetadata_GivenFieldWithSomeFlags_WhenRendered_ThenOnlySetFlagsAppear(t *testing.T) {
	field := schema.Field{
		JSONName:    "status",
		DisplayName: "Status",
		GoType:      "string",
		IsID:        false,
		Readonly:    false,
		Required:    true,
		Filterable:  true,
		EnumOptions: []string{"active", "inactive"},
		Validation:  map[string]string{"format": "enum"},
	}
	out := renderFieldMetadata([]schema.Field{field})

	for _, want := range []string{
		`Name: "status"`,
		`DisplayName: "Status"`,
		`Type: "enum"`,
		`Required: true`,
		`Filterable: true`,
		`Options: []string{"active", "inactive"}`,
		`Validation: map[string]any{"format": "enum"}`,
	} {
		if !strings.Contains(out, want) {
			t.Errorf("expected fragment missing: %s\noutput:\n%s", want, out)
		}
	}
	for _, unwanted := range []string{
		"Readonly",
		"Writeonly",
		"Sortable",
		"Searchable",
	} {
		if strings.Contains(out, unwanted+": true") {
			t.Errorf("unexpected flag %q emitted\noutput:\n%s", unwanted, out)
		}
	}
}

func TestRenderFieldMetadata_GivenPrimaryField_WhenRendered_ThenAlsoEmitsReadonly(t *testing.T) {
	out := renderFieldMetadata([]schema.Field{{JSONName: "id", GoType: "uint", IsID: true}})
	// Matching the admin package: primary keys are readonly by default so
	// UI clients don't need to special-case them.
	if !strings.Contains(out, "Primary: true, Readonly: true") {
		t.Fatalf("id field should emit both flags; got:\n%s", out)
	}
}

// --- renderValidatorBody ----------------------------------------------

func TestRenderValidatorBody_GivenEntityWithEnumAndFields_WhenRendered_ThenEmitsAllLayersInOrder(t *testing.T) {
	e := schema.Entity{
		Fields: []schema.Field{
			{GoName: "ID", JSONName: "id", GoType: "uint", IsID: true},
			{GoName: "Status", JSONName: "status", GoType: "string", EnumOptions: []string{"active", "inactive"}},
			{GoName: "Email", JSONName: "email", GoType: "string"},
		},
	}
	out := renderValidatorBody(e)

	// Enum enforcement uses the runtime helper — no inline fmt.Errorf.
	if !strings.Contains(out, `runtime.EnforceEnum("status", string(e.Status), []string{"active", "inactive"})`) {
		t.Errorf("enum enforcement missing or malformed\n%s", out)
	}
	// One CallFieldValidator per field, in declaration order.
	for _, f := range []string{"id", "status", "email"} {
		if !strings.Contains(out, `runtime.CallFieldValidator("`+f+`", e.`) {
			t.Errorf("missing field validator for %s\n%s", f, out)
		}
	}
	// Entity-level call terminates the body.
	if !strings.Contains(out, "runtime.CallEntityValidator(*e, e)") {
		t.Errorf("missing entity-level validator call\n%s", out)
	}
	if !strings.HasSuffix(strings.TrimRight(out, "\n\t "), "return nil") {
		t.Errorf("body should end with `return nil`\n%s", out)
	}
}

func TestRenderValidatorBody_GivenEntityWithoutEnumFields_WhenRendered_ThenOmitsEnforceEnum(t *testing.T) {
	e := schema.Entity{Fields: []schema.Field{{GoName: "ID", JSONName: "id", GoType: "uint", IsID: true}}}
	out := renderValidatorBody(e)
	if strings.Contains(out, "EnforceEnum") {
		t.Fatalf("should not emit EnforceEnum when no enum fields:\n%s", out)
	}
}

// --- renderCustomActions ----------------------------------------------

func TestRenderCustomActions_GivenEmptyActions_WhenRendered_ThenReturnsEmptyString(t *testing.T) {
	if got := renderCustomActions(nil); got != "" {
		t.Fatalf("want empty, got %q", got)
	}
}

func TestRenderCustomActions_GivenMixedActionAttributes_WhenRendered_ThenOptionalFieldsAppearOnlyWhenSet(t *testing.T) {
	out := renderCustomActions([]schema.CustomAction{
		{Name: "activate", DisplayName: "Activate"},
		{Name: "purge", Destructive: true},
		{Name: "resend"}, // no display, not destructive
	})
	for _, want := range []string{
		`{Name: "activate", DisplayName: "Activate"},`,
		`{Name: "purge", Destructive: true},`,
		`{Name: "resend"},`,
	} {
		if !strings.Contains(out, want) {
			t.Errorf("missing expected line: %s\noutput:\n%s", want, out)
		}
	}
}

// --- renderDefaultSort ------------------------------------------------

func TestRenderDefaultSort_GivenEmptySpecs_WhenRendered_ThenReturnsEmptyString(t *testing.T) {
	if got := renderDefaultSort(nil); got != "" {
		t.Fatalf("want empty, got %q", got)
	}
}

func TestRenderDefaultSort_GivenSingleSpec_WhenRendered_ThenEmitsSingletonSliceLiteral(t *testing.T) {
	out := renderDefaultSort([]schema.SortSpec{{Field: "created_at", Order: "desc"}})
	if out != `[]runtime.SortSpec{{Field: "created_at", Order: "desc"}}` {
		t.Fatalf("got %q", out)
	}
}

func TestRenderDefaultSort_GivenMultipleSpecs_WhenRendered_ThenEmitsCommaJoinedSliceLiteral(t *testing.T) {
	out := renderDefaultSort([]schema.SortSpec{
		{Field: "a", Order: "desc"},
		{Field: "b", Order: "asc"},
	})
	want := `[]runtime.SortSpec{{Field: "a", Order: "desc"}, {Field: "b", Order: "asc"}}`
	if out != want {
		t.Fatalf("got %q\nwant %q", out, want)
	}
}

// --- buildHandlerData -------------------------------------------------

func TestBuildHandlerData_GivenEntityWithoutPaginationOverrides_WhenBuilt_ThenAppliesFrameworkDefaults(t *testing.T) {
	e := schema.Entity{
		GoName: "User",
		Name:   "user",
		Fields: []schema.Field{{GoName: "ID", JSONName: "id", GoType: "uint", IsID: true}},
		IDGen:  "increment",
	}
	data := buildHandlerData(e, Options{OutPackage: "admin"})
	if data.EffectiveDefaultPageSize != 20 || data.EffectiveMaxPageSize != 100 {
		t.Fatalf("defaults not applied: %+v", data)
	}
	if data.SeedUUIDIDGenerator {
		t.Fatalf("non-uuid entity should not seed uuid generator")
	}
}

func TestBuildHandlerData_GivenEntityWithPaginationOverrides_WhenBuilt_ThenRespectsThem(t *testing.T) {
	e := schema.Entity{
		GoName:          "User",
		Name:            "user",
		Fields:          []schema.Field{{GoName: "ID", JSONName: "id", GoType: "uint", IsID: true}},
		IDGen:           "increment",
		DefaultPageSize: 25,
		MaxPageSize:     200,
	}
	data := buildHandlerData(e, Options{OutPackage: "admin"})
	if data.EffectiveDefaultPageSize != 25 || data.EffectiveMaxPageSize != 200 {
		t.Fatalf("overrides dropped: %+v", data)
	}
}

func TestBuildHandlerData_GivenUUIDEntity_WhenBuilt_ThenSetsUUIDFlags(t *testing.T) {
	e := schema.Entity{
		GoName: "Session",
		Name:   "session",
		Fields: []schema.Field{{GoName: "ID", JSONName: "id", GoType: "uuid.UUID", IsID: true}},
		IDGen:  "uuid",
	}
	data := buildHandlerData(e, Options{OutPackage: "admin"})
	if !data.SeedUUIDIDGenerator || !data.NeedsUUID {
		t.Fatalf("uuid flags missing: %+v", data)
	}
}
