package admin

import (
	"fmt"
	"reflect"
	"strconv"
	"strings"
	"unicode"
)

type fieldInfo struct {
	name        string
	jsonName    string
	displayName string
	index       []int
	kind        reflect.Kind
	goType      reflect.Type
	isID        bool
	filterable  bool
	sortable    bool
	searchable  bool
	readonly    bool
	writeonly   bool
	required    bool
	enumOptions []string
	validation  map[string]any
}

type entityMeta struct {
	name        string
	displayName string
	typ         reflect.Type
	idField     *fieldInfo
	fields      []*fieldInfo
	byJSON      map[string]*fieldInfo
	storage     Storage
	listConfig  ListConfig
	actions     map[string]CustomAction
	idGenerator IDGenerator
}

func newEntityMeta(model any, opts entityOptions) (*entityMeta, error) {
	t := reflect.TypeOf(model)
	if t == nil {
		return nil, fmt.Errorf("admin: nil model")
	}
	for t.Kind() == reflect.Ptr {
		t = t.Elem()
	}
	if t.Kind() != reflect.Struct {
		return nil, fmt.Errorf("admin: model must be a struct, got %s", t.Kind())
	}

	m := &entityMeta{
		name:   opts.name,
		typ:    t,
		byJSON: map[string]*fieldInfo{},
	}
	if m.name == "" {
		m.name = defaultEntityName(t.Name())
	}

	if err := m.collectFields(t, nil); err != nil {
		return nil, err
	}
	if m.idField == nil {
		return nil, fmt.Errorf("admin: entity %q has no id field (tag `admin:\"id\"` or field named ID)", m.name)
	}
	return m, nil
}

func (m *entityMeta) collectFields(t reflect.Type, parentIndex []int) error {
	for i := 0; i < t.NumField(); i++ {
		sf := t.Field(i)
		if !sf.IsExported() {
			continue
		}
		idx := append(append([]int{}, parentIndex...), i)

		if sf.Anonymous && sf.Type.Kind() == reflect.Struct {
			if err := m.collectFields(sf.Type, idx); err != nil {
				return err
			}
			continue
		}

		jsonName := sf.Name
		if tag := sf.Tag.Get("json"); tag != "" && tag != "-" {
			if comma := strings.Index(tag, ","); comma >= 0 {
				tag = tag[:comma]
			}
			if tag != "" {
				jsonName = tag
			}
		}

		fi := &fieldInfo{
			name:        sf.Name,
			jsonName:    jsonName,
			displayName: sf.Tag.Get("display"),
			index:       idx,
			kind:        sf.Type.Kind(),
			goType:      sf.Type,
		}

		adminTag := sf.Tag.Get("admin")
		for _, part := range strings.Split(adminTag, ",") {
			switch strings.TrimSpace(part) {
			case "":
			case "id":
				fi.isID = true
			case "filterable":
				fi.filterable = true
			case "sortable":
				fi.sortable = true
			case "searchable":
				fi.searchable = true
			case "readonly":
				fi.readonly = true
			case "writeonly":
				fi.writeonly = true
			case "required":
				fi.required = true
			}
		}
		if enum := sf.Tag.Get("enum"); enum != "" {
			for _, o := range strings.Split(enum, ",") {
				if o = strings.TrimSpace(o); o != "" {
					fi.enumOptions = append(fi.enumOptions, o)
				}
			}
		}
		if v := sf.Tag.Get("validate"); v != "" {
			fi.validation = parseValidation(v)
		}
		if !fi.isID && strings.EqualFold(sf.Name, "ID") {
			fi.isID = true
		}
		if fi.isID {
			if m.idField != nil {
				return fmt.Errorf("admin: entity %q has multiple id fields", m.name)
			}
			m.idField = fi
		}

		m.fields = append(m.fields, fi)
		m.byJSON[fi.jsonName] = fi
	}
	return nil
}

func (m *entityMeta) newInstance() reflect.Value {
	return reflect.New(m.typ)
}

func (m *entityMeta) getID(v reflect.Value) any {
	v = reflect.Indirect(v)
	return v.FieldByIndex(m.idField.index).Interface()
}

func (m *entityMeta) setID(v reflect.Value, id any) error {
	v = reflect.Indirect(v)
	f := v.FieldByIndex(m.idField.index)
	idv := reflect.ValueOf(id)
	if idv.Type().AssignableTo(f.Type()) {
		f.Set(idv)
		return nil
	}
	if idv.Type().ConvertibleTo(f.Type()) {
		f.Set(idv.Convert(f.Type()))
		return nil
	}
	return fmt.Errorf("cannot assign id of type %s to field of type %s", idv.Type(), f.Type())
}

func defaultEntityName(s string) string {
	if s == "" {
		return s
	}
	r := []rune(s)
	r[0] = unicode.ToLower(r[0])
	return string(r)
}

// parseValidation parses tags like "format=email,maxLength=255,minLength=2".
// Numeric values are surfaced as int, everything else as string.
func parseValidation(s string) map[string]any {
	out := map[string]any{}
	for _, part := range strings.Split(s, ",") {
		part = strings.TrimSpace(part)
		if part == "" {
			continue
		}
		eq := strings.Index(part, "=")
		if eq < 0 {
			out[part] = true
			continue
		}
		k, v := part[:eq], part[eq+1:]
		if n, err := strconv.Atoi(v); err == nil {
			out[k] = n
			continue
		}
		out[k] = v
	}
	return out
}
