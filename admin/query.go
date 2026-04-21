package admin

import (
	"fmt"
	"reflect"
	"sort"
	"strings"
)

type Filter struct {
	Field string `json:"field"`
	Op    string `json:"op"`
	Value any    `json:"value"`
}

type SortSpec struct {
	Field string `json:"field"`
	Order string `json:"order"`
}

type ListQuery struct {
	Filters  []Filter   `json:"filters"`
	Sort     []SortSpec `json:"sort"`
	Page     int        `json:"page"`
	PageSize int        `json:"page_size"`
}

type ListResult struct {
	Items    []any `json:"items"`
	Total    int   `json:"total"`
	Page     int   `json:"page"`
	PageSize int   `json:"page_size"`
}

const (
	opEq       = "eq"
	opNe       = "ne"
	opLt       = "lt"
	opLte      = "lte"
	opGt       = "gt"
	opGte      = "gte"
	opContains = "contains"
	opIn       = "in"
)

func applyQuery(m *entityMeta, items []reflect.Value, q ListQuery) (ListResult, error) {
	filtered := items
	if len(q.Filters) > 0 {
		out := make([]reflect.Value, 0, len(filtered))
		for _, it := range filtered {
			keep := true
			for _, f := range q.Filters {
				fi, ok := m.byJSON[f.Field]
				if !ok || !fi.filterable {
					return ListResult{}, badRequest(fmt.Sprintf("field %q is not filterable", f.Field))
				}
				match, err := matchFilter(reflect.Indirect(it).FieldByIndex(fi.index), f)
				if err != nil {
					return ListResult{}, err
				}
				if !match {
					keep = false
					break
				}
			}
			if keep {
				out = append(out, it)
			}
		}
		filtered = out
	}

	if len(q.Sort) > 0 {
		for _, s := range q.Sort {
			fi, ok := m.byJSON[s.Field]
			if !ok || !fi.sortable {
				return ListResult{}, badRequest(fmt.Sprintf("field %q is not sortable", s.Field))
			}
		}
		sortItems(m, filtered, q.Sort)
	}

	total := len(filtered)
	page, pageSize := q.Page, q.PageSize
	if pageSize <= 0 {
		pageSize = 20
	}
	if page <= 0 {
		page = 1
	}
	start := (page - 1) * pageSize
	end := start + pageSize
	if start > total {
		start = total
	}
	if end > total {
		end = total
	}
	slice := filtered[start:end]

	items2 := make([]any, 0, len(slice))
	for _, v := range slice {
		items2 = append(items2, v.Interface())
	}
	return ListResult{
		Items:    items2,
		Total:    total,
		Page:     page,
		PageSize: pageSize,
	}, nil
}

func matchFilter(field reflect.Value, f Filter) (bool, error) {
	switch f.Op {
	case opEq, "":
		return compare(field, f.Value) == 0, nil
	case opNe:
		return compare(field, f.Value) != 0, nil
	case opLt:
		return compare(field, f.Value) < 0, nil
	case opLte:
		return compare(field, f.Value) <= 0, nil
	case opGt:
		return compare(field, f.Value) > 0, nil
	case opGte:
		return compare(field, f.Value) >= 0, nil
	case opContains:
		if field.Kind() != reflect.String {
			return false, badRequest(fmt.Sprintf("contains op requires string field %q", f.Field))
		}
		s, _ := f.Value.(string)
		return strings.Contains(field.String(), s), nil
	case opIn:
		arr, ok := f.Value.([]any)
		if !ok {
			return false, badRequest(fmt.Sprintf("in op requires array value for field %q", f.Field))
		}
		for _, v := range arr {
			if compare(field, v) == 0 {
				return true, nil
			}
		}
		return false, nil
	default:
		return false, badRequest(fmt.Sprintf("unknown operator %q", f.Op))
	}
}

func sortItems(m *entityMeta, items []reflect.Value, specs []SortSpec) {
	sort.SliceStable(items, func(i, j int) bool {
		for _, s := range specs {
			fi := m.byJSON[s.Field]
			a := reflect.Indirect(items[i]).FieldByIndex(fi.index)
			b := reflect.Indirect(items[j]).FieldByIndex(fi.index)
			c := compareValues(a, b)
			if c == 0 {
				continue
			}
			if strings.EqualFold(s.Order, "desc") {
				return c > 0
			}
			return c < 0
		}
		return false
	})
}

func compareValues(a, b reflect.Value) int {
	switch a.Kind() {
	case reflect.String:
		return strings.Compare(a.String(), b.String())
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		ai, bi := a.Int(), b.Int()
		switch {
		case ai < bi:
			return -1
		case ai > bi:
			return 1
		}
		return 0
	case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64:
		ai, bi := a.Uint(), b.Uint()
		switch {
		case ai < bi:
			return -1
		case ai > bi:
			return 1
		}
		return 0
	case reflect.Float32, reflect.Float64:
		ai, bi := a.Float(), b.Float()
		switch {
		case ai < bi:
			return -1
		case ai > bi:
			return 1
		}
		return 0
	case reflect.Bool:
		ab, bb := a.Bool(), b.Bool()
		switch {
		case !ab && bb:
			return -1
		case ab && !bb:
			return 1
		}
		return 0
	}
	return 0
}

func compare(field reflect.Value, raw any) int {
	rv := reflect.ValueOf(raw)
	if !rv.IsValid() {
		if field.IsZero() {
			return 0
		}
		return 1
	}
	switch field.Kind() {
	case reflect.String:
		s, _ := raw.(string)
		return strings.Compare(field.String(), s)
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		fv := field.Int()
		v := toInt64(raw)
		switch {
		case fv < v:
			return -1
		case fv > v:
			return 1
		}
		return 0
	case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64:
		fv := field.Uint()
		v := uint64(toInt64(raw))
		switch {
		case fv < v:
			return -1
		case fv > v:
			return 1
		}
		return 0
	case reflect.Float32, reflect.Float64:
		fv := field.Float()
		v := toFloat64(raw)
		switch {
		case fv < v:
			return -1
		case fv > v:
			return 1
		}
		return 0
	case reflect.Bool:
		b, _ := raw.(bool)
		fb := field.Bool()
		switch {
		case !fb && b:
			return -1
		case fb && !b:
			return 1
		}
		return 0
	}
	return 0
}

func toInt64(v any) int64 {
	switch n := v.(type) {
	case int:
		return int64(n)
	case int64:
		return n
	case int32:
		return int64(n)
	case float64:
		return int64(n)
	case float32:
		return int64(n)
	}
	return 0
}

func toFloat64(v any) float64 {
	switch n := v.(type) {
	case float64:
		return n
	case float32:
		return float64(n)
	case int:
		return float64(n)
	case int64:
		return float64(n)
	}
	return 0
}
