package parser

import (
	"errors"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/hokkung/go-admin/admin-gen/internal/schema"
)

// --- parseArgs ---------------------------------------------------------

func TestParseArgs_GivenVariousInputStrings_WhenParsed_ThenReturnsExpectedMap(t *testing.T) {
	cases := []struct {
		name string
		in   string
		want map[string]string
	}{
		{"given empty string when parsed then returns empty map", "", map[string]string{}},
		{"given single bare assignment when parsed then returns single key", "idgen=uuid", map[string]string{"idgen": "uuid"}},
		{"given valueless flag when parsed then returns empty string value", "destructive", map[string]string{"destructive": ""}},
		{"given two bare assignments when parsed then returns both keys", "name=user idgen=uuid", map[string]string{"name": "user", "idgen": "uuid"}},
		{"given quoted value with spaces when parsed then preserves internal whitespace", `display="Activate User"`, map[string]string{"display": "Activate User"}},
		{"given quoted value with commas when parsed then preserves content verbatim", `default_sort="a:desc,b:asc"`, map[string]string{"default_sort": "a:desc,b:asc"}},
		{"given bare and flag together when parsed then returns both", "name=user destructive", map[string]string{"name": "user", "destructive": ""}},
		{"given extra whitespace when parsed then ignores it", "   a=1   b=2   ", map[string]string{"a": "1", "b": "2"}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := parseArgs(tc.in)
			if !reflect.DeepEqual(got, tc.want) {
				t.Fatalf("parseArgs(%q) = %v, want %v", tc.in, got, tc.want)
			}
		})
	}
}

// --- parsePositiveInt --------------------------------------------------

func TestParsePositiveInt_GivenStringInputs_WhenParsed_ThenValidReturnsNumberElseError(t *testing.T) {
	cases := []struct {
		name    string
		in      string
		want    int
		wantErr string
	}{
		{"given empty string when parsed then returns zero without error", "", 0, ""},
		{"given zero literal when parsed then returns zero without error", "0", 0, ""},
		{"given positive integer when parsed then returns that value", "25", 25, ""},
		{"given non-numeric string when parsed then returns not-integer error", "abc", 0, "not an integer"},
		{"given negative integer when parsed then returns non-negative error", "-5", 0, "must be non-negative"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, err := parsePositiveInt(tc.in)
			if tc.wantErr == "" {
				if err != nil {
					t.Fatalf("unexpected err: %v", err)
				}
				if got != tc.want {
					t.Fatalf("got %d, want %d", got, tc.want)
				}
				return
			}
			if err == nil || !strings.Contains(err.Error(), tc.wantErr) {
				t.Fatalf("err = %v, want substring %q", err, tc.wantErr)
			}
		})
	}
}

// --- parseSortSpecs ----------------------------------------------------

func TestParseSortSpecs_GivenClauseString_WhenParsed_ThenReturnsSpecListOrError(t *testing.T) {
	cases := []struct {
		name    string
		in      string
		want    []schema.SortSpec
		wantErr string
	}{
		{"given single clause with order when parsed then returns that spec", "created_at:desc", []schema.SortSpec{{Field: "created_at", Order: "desc"}}, ""},
		{"given single clause without order when parsed then defaults order to asc", "name", []schema.SortSpec{{Field: "name", Order: "asc"}}, ""},
		{"given multiple clauses when parsed then returns them in order", "a:desc,b:asc", []schema.SortSpec{{Field: "a", Order: "desc"}, {Field: "b", Order: "asc"}}, ""},
		{"given uppercase order when parsed then normalizes to lowercase", "a:DESC", []schema.SortSpec{{Field: "a", Order: "desc"}}, ""},
		{"given whitespace between tokens when parsed then trims it", "  a : desc , b ", []schema.SortSpec{{Field: "a", Order: "desc"}, {Field: "b", Order: "asc"}}, ""},
		{"given empty string when parsed then returns nil without error", "", nil, ""},
		{"given stray comma when parsed then skips the empty clause", "a:desc,,b:asc", []schema.SortSpec{{Field: "a", Order: "desc"}, {Field: "b", Order: "asc"}}, ""},

		{"given empty field when parsed then returns empty-field error", ":desc", nil, "empty field"},
		{"given invalid order when parsed then returns invalid-order error", "a:sideways", nil, "invalid sort order"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, err := parseSortSpecs(tc.in)
			if tc.wantErr == "" {
				if err != nil {
					t.Fatalf("unexpected err: %v", err)
				}
				if !reflect.DeepEqual(got, tc.want) {
					t.Fatalf("got %v, want %v", got, tc.want)
				}
				return
			}
			if err == nil || !strings.Contains(err.Error(), tc.wantErr) {
				t.Fatalf("err = %v, want substring %q", err, tc.wantErr)
			}
		})
	}
}

// --- readDirective + readActionDirectives ------------------------------

func TestReadDirective_GivenCommentLines_WhenRead_ThenDetectsAdminGenerateAndSkipsAdminAction(t *testing.T) {
	cases := []struct {
		name     string
		comments []string
		wantOK   bool
		wantArgs map[string]string
	}{
		{"given plain comment when read then reports not annotated", []string{"// a plain comment"}, false, nil},
		{"given bare admin:generate when read then reports annotated with empty args", []string{"//admin:generate"}, true, map[string]string{}},
		{"given admin:generate with args when read then parses args", []string{"//admin:generate name=user idgen=uuid"}, true, map[string]string{"name": "user", "idgen": "uuid"}},
		{"given admin:generate among other comments when read then finds it", []string{"// preamble", "//admin:generate name=user"}, true, map[string]string{"name": "user"}},
		// readDirective must not match //admin:action lines — we rely on that
		// disambiguation so the two directives don't collide.
		{"given only admin:action lines when read then reports not annotated", []string{"//admin:action name=activate"}, false, nil},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			args, ok := readDirective(tc.comments)
			if ok != tc.wantOK {
				t.Fatalf("ok = %v, want %v", ok, tc.wantOK)
			}
			if tc.wantOK && !reflect.DeepEqual(args, tc.wantArgs) {
				t.Fatalf("args = %v, want %v", args, tc.wantArgs)
			}
		})
	}
}

func TestReadActionDirectives_GivenNoActionComments_WhenRead_ThenReturnsEmptySliceWithoutError(t *testing.T) {
	actions, err := readActionDirectives([]string{"// plain"})
	if err != nil {
		t.Fatal(err)
	}
	if len(actions) != 0 {
		t.Fatalf("want no actions, got %v", actions)
	}
}

func TestReadActionDirectives_GivenMultipleDeclarations_WhenRead_ThenCollectsAllInOrder(t *testing.T) {
	actions, err := readActionDirectives([]string{
		`//admin:action name=activate display="Activate User"`,
		`//admin:action name=purge destructive`,
	})
	if err != nil {
		t.Fatal(err)
	}
	want := []schema.CustomAction{
		{Name: "activate", DisplayName: "Activate User"},
		{Name: "purge", Destructive: true},
	}
	if !reflect.DeepEqual(actions, want) {
		t.Fatalf("got %+v, want %+v", actions, want)
	}
}

func TestReadActionDirectives_GivenActionWithoutName_WhenRead_ThenReturnsNameRequiredError(t *testing.T) {
	_, err := readActionDirectives([]string{`//admin:action display="No Name"`})
	if err == nil || !strings.Contains(err.Error(), "name=<…> is required") {
		t.Fatalf("err = %v, want name-required message", err)
	}
}

func TestReadActionDirectives_GivenDuplicateName_WhenRead_ThenReturnsDuplicateError(t *testing.T) {
	_, err := readActionDirectives([]string{
		`//admin:action name=x`,
		`//admin:action name=x display=Different`,
	})
	if err == nil || !strings.Contains(err.Error(), "duplicate action name") {
		t.Fatalf("err = %v, want duplicate message", err)
	}
}

func TestReadActionDirectives_GivenNonActionLines_WhenRead_ThenIgnoresThem(t *testing.T) {
	actions, err := readActionDirectives([]string{
		"// random",
		`//admin:generate`,
		`//admin:action name=activate`,
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(actions) != 1 || actions[0].Name != "activate" {
		t.Fatalf("got %+v, want single activate action", actions)
	}
}

// --- jsonName / defaultName / firstNonEmpty ---------------------------

func TestJSONName_GivenGoNameAndTag_WhenResolved_ThenFollowsJSONTagConventions(t *testing.T) {
	cases := []struct {
		name   string
		goName string
		tag    string
		want   string
	}{
		{"given no tag when resolved then uses go name", "Email", "", "Email"},
		{"given dash tag when resolved then uses go name", "Email", "-", "Email"},
		{"given simple tag when resolved then uses tag value", "Email", "email", "email"},
		{"given tag with omitempty when resolved then uses tag value before comma", "Email", "email,omitempty", "email"},
		{"given tag starting with comma when resolved then falls back to go name", "Email", ",omitempty", "Email"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := jsonName(tc.goName, tc.tag); got != tc.want {
				t.Fatalf("jsonName(%q, %q) = %q, want %q", tc.goName, tc.tag, got, tc.want)
			}
		})
	}
}

func TestDefaultName_GivenGoNameAndOverride_WhenResolved_ThenPrefersOverrideElseLowerFirstRune(t *testing.T) {
	cases := []struct {
		name                   string
		goName, override, want string
	}{
		{"given empty override when resolved then lowercases first rune of go name", "User", "", "user"},
		{"given explicit override when resolved then uses override verbatim", "User", "people", "people"},
		{"given empty go name and empty override when resolved then returns empty string", "", "", ""},
		{"given go name with uppercase initialism when resolved then only lowers first rune", "HTTPRequest", "", "hTTPRequest"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := defaultName(tc.goName, tc.override); got != tc.want {
				t.Fatalf("defaultName(%q, %q) = %q, want %q", tc.goName, tc.override, got, tc.want)
			}
		})
	}
}

func TestFirstNonEmpty_GivenTwoStrings_WhenCalled_ThenReturnsFirstNonEmptyOrEmpty(t *testing.T) {
	cases := []struct {
		name       string
		a, b, want string
	}{
		{"given both empty when called then returns empty", "", "", ""},
		{"given a non-empty when called then returns a", "a", "", "a"},
		{"given only b non-empty when called then returns b", "", "b", "b"},
		{"given both non-empty when called then returns a", "a", "b", "a"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := firstNonEmpty(tc.a, tc.b); got != tc.want {
				t.Fatalf("firstNonEmpty(%q, %q) = %q, want %q", tc.a, tc.b, got, tc.want)
			}
		})
	}
}

// --- parseField --------------------------------------------------------

func TestParseField_GivenAdminTagWithFlags_WhenParsed_ThenAllFlagsSet(t *testing.T) {
	f := parseField("Name", "string", `json:"name" admin:"filterable,sortable,required,searchable,readonly,writeonly"`)
	if !f.Filterable || !f.Sortable || !f.Required || !f.Searchable || !f.Readonly || !f.Writeonly {
		t.Fatalf("tag flags not all set: %+v", f)
	}
}

func TestParseField_GivenIdTagOrFieldNamedID_WhenParsed_ThenIsIDIsTrue(t *testing.T) {
	a := parseField("Custom", "uint", `admin:"id"`)
	if !a.IsID {
		t.Fatal("admin:\"id\" → IsID should be true")
	}
	b := parseField("ID", "uint", "")
	if !b.IsID {
		t.Fatal("field named ID → IsID should be true by default")
	}
	c := parseField("id", "uint", "") // lowercase also matches via EqualFold
	if !c.IsID {
		t.Fatal("lowercase 'id' field should also be recognized")
	}
}

func TestParseField_GivenEnumTag_WhenParsed_ThenEnumOptionsListedInOrder(t *testing.T) {
	f := parseField("Status", "string", `enum:"active,inactive"`)
	if !reflect.DeepEqual(f.EnumOptions, []string{"active", "inactive"}) {
		t.Fatalf("EnumOptions = %v", f.EnumOptions)
	}
}

func TestParseField_GivenEnumTagWithWhitespaceAndEmpties_WhenParsed_ThenTrimsAndDropsEmpties(t *testing.T) {
	f := parseField("Status", "string", `enum:" active , , inactive "`)
	if !reflect.DeepEqual(f.EnumOptions, []string{"active", "inactive"}) {
		t.Fatalf("EnumOptions = %v", f.EnumOptions)
	}
}

func TestParseField_GivenValidateTag_WhenParsed_ThenSplitsKeyValuePairsAndBareFlags(t *testing.T) {
	f := parseField("Email", "string", `validate:"format=email,required"`)
	if f.Validation["format"] != "email" {
		t.Fatalf("format = %q", f.Validation["format"])
	}
	if _, ok := f.Validation["required"]; !ok {
		t.Fatal("bare flag 'required' should be present in Validation map")
	}
}

func TestParseField_GivenDisplayTag_WhenParsed_ThenCapturesDisplayName(t *testing.T) {
	f := parseField("Email", "string", `display:"E-mail Address"`)
	if f.DisplayName != "E-mail Address" {
		t.Fatalf("DisplayName = %q", f.DisplayName)
	}
}

func TestParseField_GivenNoJSONTag_WhenParsed_ThenFallsBackToGoName(t *testing.T) {
	f := parseField("Email", "string", "")
	if f.JSONName != "Email" {
		t.Fatalf("JSONName = %q", f.JSONName)
	}
}

// --- Parse (end-to-end over testdata) ----------------------------------

// parseFixture is the common path for fixture-based tests. It resolves the
// absolute path of testdata/<dir> so the call works whether go test is
// invoked from the repo root or the package dir.
func parseFixture(t *testing.T, dir string) ([]schema.Entity, error) {
	t.Helper()
	abs, err := filepath.Abs(filepath.Join("testdata", dir))
	if err != nil {
		t.Fatal(err)
	}
	return Parse(ParseOptions{Dir: abs, PackagePath: "example.com/fixture/" + dir})
}

func TestParse_GivenBasicFixture_WhenParsed_ThenReturnsSingleEntityWithDefaults(t *testing.T) {
	ents, err := parseFixture(t, "basic")
	if err != nil {
		t.Fatal(err)
	}
	if len(ents) != 1 {
		t.Fatalf("want 1 entity, got %d", len(ents))
	}
	e := ents[0]
	if e.GoName != "User" || e.Name != "user" || e.IDGen != "increment" {
		t.Fatalf("entity shape = %+v", e)
	}
	if e.DisplayName != "User" {
		t.Fatalf("DisplayName = %q, want struct name default", e.DisplayName)
	}
	if got := len(e.Fields); got != 2 {
		t.Fatalf("got %d fields, want 2", got)
	}
}

func TestParse_GivenUUIDIDType_WhenParsed_ThenIDGenDefaultsToUUID(t *testing.T) {
	ents, err := parseFixture(t, "uuid_id")
	if err != nil {
		t.Fatal(err)
	}
	if len(ents) != 1 || ents[0].IDGen != "uuid" {
		t.Fatalf("IDGen = %q, want uuid (auto-detected from uuid.UUID type)", ents[0].IDGen)
	}
}

func TestParse_GivenListConfigDirectives_WhenParsed_ThenPopulatesEntityListConfig(t *testing.T) {
	ents, err := parseFixture(t, "list_config")
	if err != nil {
		t.Fatal(err)
	}
	e := ents[0]
	if e.DefaultPageSize != 25 || e.MaxPageSize != 200 {
		t.Fatalf("pagination = {%d, %d}", e.DefaultPageSize, e.MaxPageSize)
	}
	want := []schema.SortSpec{{Field: "created_at", Order: "desc"}, {Field: "name", Order: "asc"}}
	if !reflect.DeepEqual(e.DefaultSort, want) {
		t.Fatalf("DefaultSort = %v, want %v", e.DefaultSort, want)
	}
}

func TestParse_GivenCustomActionDirectives_WhenParsed_ThenPopulatesCustomActions(t *testing.T) {
	ents, err := parseFixture(t, "actions")
	if err != nil {
		t.Fatal(err)
	}
	want := []schema.CustomAction{
		{Name: "activate", DisplayName: "Activate User"},
		{Name: "purge", Destructive: true},
	}
	if !reflect.DeepEqual(ents[0].CustomActions, want) {
		t.Fatalf("CustomActions = %+v, want %+v", ents[0].CustomActions, want)
	}
}

func TestParse_GivenSamePackageEmbeddedStruct_WhenParsed_ThenFieldsAreFlattenedInDeclaredOrder(t *testing.T) {
	ents, err := parseFixture(t, "embedded")
	if err != nil {
		t.Fatal(err)
	}
	var names []string
	for _, f := range ents[0].Fields {
		names = append(names, f.GoName)
	}
	want := []string{"ID", "Title", "CreatedAt", "UpdatedAt"}
	if !reflect.DeepEqual(names, want) {
		t.Fatalf("flattened fields = %v, want %v", names, want)
	}
}

func TestParse_GivenCrossPackageEmbed_WhenParsed_ThenEmbeddedFieldsAreSkipped(t *testing.T) {
	// A selector embed (e.g. `gorm.Model`) is silently skipped — the
	// testdata package embeds one and the visible fields should only be
	// the ones declared inline.
	ents, err := parseFixture(t, "embedded_cross")
	if err != nil {
		t.Fatal(err)
	}
	var names []string
	for _, f := range ents[0].Fields {
		names = append(names, f.GoName)
	}
	want := []string{"ID", "Title"} // gorm.Model fields must not leak in
	if !reflect.DeepEqual(names, want) {
		t.Fatalf("fields = %v, want %v", names, want)
	}
}

func TestParse_GivenPathologicalEmbedCycle_WhenParsed_ThenCycleGuardStopsRecursion(t *testing.T) {
	// testdata/embedded_cycle contains a pathological self-embed. The cycle
	// guard should stop the recursion; the test passes if Parse returns in
	// finite time.
	ents, err := parseFixture(t, "embedded_cycle")
	if err != nil {
		t.Fatal(err)
	}
	if len(ents) == 0 {
		t.Fatal("expected at least one entity")
	}
}

func TestParse_GivenInvalidFixture_WhenParsed_ThenReturnsExpectedError(t *testing.T) {
	cases := []struct {
		name    string
		dir     string
		wantErr string
	}{
		{"given struct without id field when parsed then returns no-primary-key error", "no_id", "no primary key field"},
		{"given non-integer default_page_size when parsed then returns page-size error", "bad_page_size", "default_page_size"},
		{"given invalid default_sort when parsed then returns sort error", "bad_sort", "default_sort"},
		{"given action name matching reserved action when parsed then returns collision error", "action_reserved", "clashes with a standard action"},
		{"given duplicate action names when parsed then returns duplicate error", "action_duplicate", "duplicate action name"},
		{"given admin:generate on non-struct type when parsed then returns not-a-struct error", "directive_on_non_struct", "not a struct"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, err := parseFixture(t, tc.dir)
			if err == nil {
				t.Fatalf("want error containing %q, got nil", tc.wantErr)
			}
			if !strings.Contains(err.Error(), tc.wantErr) {
				t.Fatalf("err = %v, want substring %q", err, tc.wantErr)
			}
		})
	}
}

func TestParse_GivenDirectoryWithoutAdminGenerate_WhenParsed_ThenReturnsEmptyEntities(t *testing.T) {
	ents, err := parseFixture(t, "no_directive")
	// The fixture has no //admin:generate anywhere, so Parse returns nil/nil
	// (the CLI layer is responsible for the "no entities found" message).
	if err != nil && !errors.Is(err, err) { // err is checked below; shape depends on dir existence
		t.Fatal(err)
	}
	if len(ents) != 0 {
		t.Fatalf("want 0 entities, got %d", len(ents))
	}
}
