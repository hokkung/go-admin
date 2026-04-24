// Package parser walks a Go package and extracts the entities the user has
// annotated with //admin:generate. The contract is deliberately minimal:
// only struct type declarations preceded by the directive are considered.
package parser

import (
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"reflect"
	"strconv"
	"strings"

	"github.com/hokkung/go-admin/admin-gen/internal/schema"
)

const (
	directive       = "//admin:generate"
	actionDirective = "//admin:action"
)

// reservedActions are the standard action names the generator already emits.
// Custom actions must not collide — checked at parse time so the error
// surfaces before codegen runs, matching the runtime admin package.
var reservedActions = map[string]struct{}{
	"create":   {},
	"get":      {},
	"update":   {},
	"delete":   {},
	"list":     {},
	"metadata": {},
}

// ParseOptions configures a parser run.
type ParseOptions struct {
	// Dir is the directory of the package to scan, e.g. "./internal/models".
	Dir string
	// PackagePath is the full import path of that package, e.g.
	// "example.com/app/internal/models". Templates need it to emit imports.
	PackagePath string
}

// Parse returns every //admin:generate-annotated struct in dir as an Entity.
func Parse(opts ParseOptions) ([]schema.Entity, error) {
	absDir, err := filepath.Abs(opts.Dir)
	if err != nil {
		return nil, fmt.Errorf("resolve dir: %w", err)
	}
	if _, err := os.Stat(absDir); err != nil {
		return nil, fmt.Errorf("stat dir: %w", err)
	}

	fset := token.NewFileSet()
	pkgs, err := parser.ParseDir(fset, absDir, func(fi os.FileInfo) bool {
		n := fi.Name()
		return !strings.HasSuffix(n, "_test.go") && strings.HasSuffix(n, ".go")
	}, parser.ParseComments)
	if err != nil {
		return nil, fmt.Errorf("parse dir: %w", err)
	}

	// Pre-index every struct type declared across the scanned files so we
	// can resolve embedded anonymous fields by name. Cross-package embeds
	// (e.g. `gorm.Model`) stay unresolved — use go/types if we ever need
	// that — and are silently skipped, matching the previous behavior.
	localStructs := map[string]*ast.StructType{}
	for _, pkg := range pkgs {
		for _, file := range pkg.Files {
			collectStructs(file, localStructs)
		}
	}

	var entities []schema.Entity
	for pkgName, pkg := range pkgs {
		for _, file := range pkg.Files {
			found, err := extractFromFile(file, pkgName, opts.PackagePath, localStructs)
			if err != nil {
				return nil, err
			}
			entities = append(entities, found...)
		}
	}
	return entities, nil
}

func collectStructs(file *ast.File, out map[string]*ast.StructType) {
	for _, decl := range file.Decls {
		gen, ok := decl.(*ast.GenDecl)
		if !ok || gen.Tok != token.TYPE {
			continue
		}
		for _, spec := range gen.Specs {
			ts, ok := spec.(*ast.TypeSpec)
			if !ok {
				continue
			}
			if st, ok := ts.Type.(*ast.StructType); ok {
				out[ts.Name.Name] = st
			}
		}
	}
}

func extractFromFile(file *ast.File, pkgName, pkgPath string, localStructs map[string]*ast.StructType) ([]schema.Entity, error) {
	var out []schema.Entity
	for _, decl := range file.Decls {
		gen, ok := decl.(*ast.GenDecl)
		if !ok || gen.Tok != token.TYPE {
			continue
		}
		// The directive lives either on the GenDecl's doc or, when the decl is
		// a parenthesized group, on each TypeSpec. Walk both.
		for _, spec := range gen.Specs {
			ts, ok := spec.(*ast.TypeSpec)
			if !ok {
				continue
			}
			comments := joinComments(gen.Doc, ts.Doc)
			args, annotated := readDirective(comments)
			if !annotated {
				continue
			}
			st, ok := ts.Type.(*ast.StructType)
			if !ok {
				return nil, fmt.Errorf("%s: //admin:generate on %s but it is not a struct", pkgName, ts.Name.Name)
			}
			actions, err := readActionDirectives(comments)
			if err != nil {
				return nil, fmt.Errorf("%s.%s: %w", pkgName, ts.Name.Name, err)
			}
			e, err := entityFromStruct(ts.Name.Name, pkgPath, pkgName, args, actions, st, localStructs)
			if err != nil {
				return nil, fmt.Errorf("%s.%s: %w", pkgName, ts.Name.Name, err)
			}
			out = append(out, e)
		}
	}
	return out, nil
}

func joinComments(groups ...*ast.CommentGroup) []string {
	var out []string
	for _, g := range groups {
		if g == nil {
			continue
		}
		for _, c := range g.List {
			out = append(out, c.Text)
		}
	}
	return out
}

// readDirective looks for `//admin:generate key=val key2="val with spaces"`
// among comment lines and returns the parsed args plus a presence bool.
func readDirective(comments []string) (map[string]string, bool) {
	for _, line := range comments {
		trimmed := strings.TrimSpace(line)
		if !strings.HasPrefix(trimmed, directive) {
			continue
		}
		// Guard against `//admin:action` being picked up by prefix match.
		rest := trimmed[len(directive):]
		if len(rest) > 0 && rest[0] != ' ' && rest[0] != '\t' {
			continue
		}
		return parseArgs(strings.TrimSpace(rest)), true
	}
	return nil, false
}

// readActionDirectives collects every `//admin:action name=… display=… destructive`
// line above a struct. Name is required; everything else is optional. Duplicate
// names are rejected to surface typos early.
func readActionDirectives(comments []string) ([]schema.CustomAction, error) {
	var out []schema.CustomAction
	seen := map[string]struct{}{}
	for _, line := range comments {
		trimmed := strings.TrimSpace(line)
		if !strings.HasPrefix(trimmed, actionDirective) {
			continue
		}
		rest := trimmed[len(actionDirective):]
		if len(rest) > 0 && rest[0] != ' ' && rest[0] != '\t' {
			continue
		}
		args := parseArgs(strings.TrimSpace(rest))
		name := args["name"]
		if name == "" {
			return nil, fmt.Errorf("%s: name=<…> is required", actionDirective)
		}
		if _, dup := seen[name]; dup {
			return nil, fmt.Errorf("%s: duplicate action name %q", actionDirective, name)
		}
		seen[name] = struct{}{}
		_, destructive := args["destructive"]
		out = append(out, schema.CustomAction{
			Name:        name,
			DisplayName: args["display"],
			Destructive: destructive,
		})
	}
	return out, nil
}

func parseArgs(s string) map[string]string {
	out := map[string]string{}
	i := 0
	for i < len(s) {
		for i < len(s) && s[i] == ' ' {
			i++
		}
		if i >= len(s) {
			break
		}
		// key
		keyStart := i
		for i < len(s) && s[i] != '=' && s[i] != ' ' {
			i++
		}
		key := s[keyStart:i]
		if i >= len(s) || s[i] != '=' {
			// valueless flag
			out[key] = ""
			continue
		}
		i++ // skip '='
		// value — quoted or bare
		if i < len(s) && s[i] == '"' {
			i++
			valStart := i
			for i < len(s) && s[i] != '"' {
				i++
			}
			out[key] = s[valStart:i]
			if i < len(s) {
				i++ // closing quote
			}
		} else {
			valStart := i
			for i < len(s) && s[i] != ' ' {
				i++
			}
			out[key] = s[valStart:i]
		}
	}
	return out
}

func entityFromStruct(goName, pkgPath, pkgName string, args map[string]string, actions []schema.CustomAction, st *ast.StructType, localStructs map[string]*ast.StructType) (schema.Entity, error) {
	e := schema.Entity{
		GoName:        goName,
		PackagePath:   pkgPath,
		PackageName:   pkgName,
		Name:          defaultName(goName, args["name"]),
		DisplayName:   firstNonEmpty(args["display"], goName),
		IDGen:         args["idgen"],
		CustomActions: actions,
	}
	// Guard against action names colliding with standard CRUD + metadata.
	// Same collision check the runtime admin package does at Register time.
	for _, a := range actions {
		if _, clash := reservedActions[a.Name]; clash {
			return schema.Entity{}, fmt.Errorf("custom action %q clashes with a standard action", a.Name)
		}
	}

	if n, err := parsePositiveInt(args["default_page_size"]); err != nil {
		return schema.Entity{}, fmt.Errorf("default_page_size: %w", err)
	} else {
		e.DefaultPageSize = n
	}
	if n, err := parsePositiveInt(args["max_page_size"]); err != nil {
		return schema.Entity{}, fmt.Errorf("max_page_size: %w", err)
	} else {
		e.MaxPageSize = n
	}
	if s := args["default_sort"]; s != "" {
		specs, err := parseSortSpecs(s)
		if err != nil {
			return schema.Entity{}, fmt.Errorf("default_sort: %w", err)
		}
		e.DefaultSort = specs
	}

	fields, err := collectFields(st, localStructs, map[string]struct{}{})
	if err != nil {
		return schema.Entity{}, err
	}
	e.Fields = fields

	if e.IDField() == nil {
		return schema.Entity{}, fmt.Errorf("no primary key field (tag admin:\"id\" or named ID)")
	}

	// Apply sensible default for IDGen based on id type.
	if e.IDGen == "" {
		idField := e.IDField()
		if idField.GoType == "uuid.UUID" {
			e.IDGen = "uuid"
		} else {
			e.IDGen = "increment"
		}
	}
	return e, nil
}

// collectFields walks st and any anonymous-embedded structs (resolved via
// localStructs) and returns the flattened field list. visited guards against
// pathological embed cycles — struct recursion isn't legal Go, but a user
// could accidentally write one and we'd prefer a stack overflow not to be
// the failure mode.
func collectFields(st *ast.StructType, localStructs map[string]*ast.StructType, visited map[string]struct{}) ([]schema.Field, error) {
	var out []schema.Field
	for _, fld := range st.Fields.List {
		if len(fld.Names) == 0 {
			// Anonymous / embedded field. Resolve same-package struct embeds;
			// cross-package embeds (selectors like `gorm.Model`) stay
			// unresolved — supporting those needs go/types.
			ident, ok := fld.Type.(*ast.Ident)
			if !ok {
				continue
			}
			embedded, ok := localStructs[ident.Name]
			if !ok {
				continue
			}
			if _, seen := visited[ident.Name]; seen {
				continue
			}
			visited[ident.Name] = struct{}{}
			nested, err := collectFields(embedded, localStructs, visited)
			if err != nil {
				return nil, err
			}
			delete(visited, ident.Name)
			out = append(out, nested...)
			continue
		}
		goType := exprString(fld.Type)
		var tag string
		if fld.Tag != nil {
			tag = unquote(fld.Tag.Value)
		}
		stag := reflect.StructTag(tag)
		for _, nm := range fld.Names {
			if !nm.IsExported() {
				continue
			}
			out = append(out, parseField(nm.Name, goType, stag))
		}
	}
	return out, nil
}

func parsePositiveInt(s string) (int, error) {
	if s == "" {
		return 0, nil
	}
	n, err := strconv.Atoi(s)
	if err != nil {
		return 0, fmt.Errorf("not an integer: %q", s)
	}
	if n < 0 {
		return 0, fmt.Errorf("must be non-negative: %d", n)
	}
	return n, nil
}

// parseSortSpecs accepts "field:desc,field2:asc" and returns the spec list.
// Order may be omitted and defaults to "asc". Commas separate clauses.
func parseSortSpecs(s string) ([]schema.SortSpec, error) {
	var out []schema.SortSpec
	for _, part := range strings.Split(s, ",") {
		part = strings.TrimSpace(part)
		if part == "" {
			continue
		}
		field, order := part, "asc"
		if colon := strings.Index(part, ":"); colon >= 0 {
			field = strings.TrimSpace(part[:colon])
			order = strings.ToLower(strings.TrimSpace(part[colon+1:]))
		}
		if field == "" {
			return nil, fmt.Errorf("empty field in sort clause %q", part)
		}
		if order != "asc" && order != "desc" {
			return nil, fmt.Errorf("invalid sort order %q (want asc|desc)", order)
		}
		out = append(out, schema.SortSpec{Field: field, Order: order})
	}
	return out, nil
}

func parseField(name, goType string, tag reflect.StructTag) schema.Field {
	f := schema.Field{
		GoName:      name,
		GoType:      goType,
		JSONName:    jsonName(name, tag.Get("json")),
		DisplayName: tag.Get("display"),
	}
	for _, p := range strings.Split(tag.Get("admin"), ",") {
		switch strings.TrimSpace(p) {
		case "id":
			f.IsID = true
		case "filterable":
			f.Filterable = true
		case "sortable":
			f.Sortable = true
		case "searchable":
			f.Searchable = true
		case "readonly":
			f.Readonly = true
		case "writeonly":
			f.Writeonly = true
		case "required":
			f.Required = true
		}
	}
	if !f.IsID && strings.EqualFold(name, "ID") {
		f.IsID = true
	}
	if enum := tag.Get("enum"); enum != "" {
		for _, opt := range strings.Split(enum, ",") {
			if opt = strings.TrimSpace(opt); opt != "" {
				f.EnumOptions = append(f.EnumOptions, opt)
			}
		}
	}
	if v := tag.Get("validate"); v != "" {
		f.Validation = map[string]string{}
		for _, part := range strings.Split(v, ",") {
			part = strings.TrimSpace(part)
			if part == "" {
				continue
			}
			if eq := strings.Index(part, "="); eq >= 0 {
				f.Validation[part[:eq]] = part[eq+1:]
			} else {
				f.Validation[part] = ""
			}
		}
	}
	return f
}

func jsonName(goName, tag string) string {
	if tag == "" || tag == "-" {
		return goName
	}
	if comma := strings.Index(tag, ","); comma >= 0 {
		tag = tag[:comma]
	}
	if tag == "" {
		return goName
	}
	return tag
}

func defaultName(goName, override string) string {
	if override != "" {
		return override
	}
	if goName == "" {
		return ""
	}
	return strings.ToLower(goName[:1]) + goName[1:]
}

func firstNonEmpty(a, b string) string {
	if a != "" {
		return a
	}
	return b
}

// exprString is a tiny formatter for ast.Expr; for the types admin-gen
// supports (idents, selectors, simple slices/arrays/pointers) it produces
// exactly what appears in source. We avoid go/types + go/importer so the
// tool stays fast and has no build-env coupling.
func exprString(e ast.Expr) string {
	switch v := e.(type) {
	case *ast.Ident:
		return v.Name
	case *ast.SelectorExpr:
		return exprString(v.X) + "." + v.Sel.Name
	case *ast.StarExpr:
		return "*" + exprString(v.X)
	case *ast.ArrayType:
		if v.Len == nil {
			return "[]" + exprString(v.Elt)
		}
		return "[" + exprString(v.Len) + "]" + exprString(v.Elt)
	case *ast.MapType:
		return "map[" + exprString(v.Key) + "]" + exprString(v.Value)
	case *ast.BasicLit:
		return v.Value
	}
	return fmt.Sprintf("%T", e)
}

func unquote(s string) string {
	if len(s) >= 2 && s[0] == '`' && s[len(s)-1] == '`' {
		return s[1 : len(s)-1]
	}
	if len(s) >= 2 && s[0] == '"' && s[len(s)-1] == '"' {
		return s[1 : len(s)-1]
	}
	return s
}
