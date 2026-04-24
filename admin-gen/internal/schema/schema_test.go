package schema

import (
	"reflect"
	"testing"
)

func TestEntity_IDField_GivenFields_WhenCalled_ThenReturnsPointerToFirstIsIDMatchOrNil(t *testing.T) {
	t.Run("given fields with IsID on B when called then returns B", func(t *testing.T) {
		e := Entity{Fields: []Field{
			{GoName: "A"},
			{GoName: "B", IsID: true},
			{GoName: "C"},
		}}
		f := e.IDField()
		if f == nil || f.GoName != "B" {
			t.Fatalf("IDField() = %+v, want B", f)
		}
	})

	t.Run("given no field flagged IsID when called then returns nil", func(t *testing.T) {
		e := Entity{Fields: []Field{{GoName: "A"}}}
		if got := e.IDField(); got != nil {
			t.Fatalf("IDField() = %+v, want nil", got)
		}
	})

	t.Run("given multiple IsID fields when called then returns the first (parser rejects duplicates upstream)", func(t *testing.T) {
		e := Entity{Fields: []Field{
			{GoName: "A", IsID: true},
			{GoName: "B", IsID: true},
		}}
		f := e.IDField()
		if f == nil || f.GoName != "A" {
			t.Fatalf("IDField() = %+v, want A", f)
		}
	})

	t.Run("given IDField pointer when mutated then change propagates to backing slice", func(t *testing.T) {
		e := Entity{Fields: []Field{{GoName: "A", IsID: true}}}
		e.IDField().DisplayName = "Primary Key"
		if e.Fields[0].DisplayName != "Primary Key" {
			t.Fatalf("IDField mutations did not propagate to Fields")
		}
	})
}

func TestEntity_Filterable_GivenFields_WhenCalled_ThenReturnsFieldsFlaggedFilterableInOrder(t *testing.T) {
	cases := []struct {
		name string
		in   []Field
		want []string // GoNames of selected fields, in order
	}{
		{"given no filterable fields when called then returns nil", []Field{{GoName: "A"}, {GoName: "B"}}, nil},
		{"given mixed fields when called then returns only filterable ones in order", []Field{
			{GoName: "A", Filterable: true},
			{GoName: "B"},
			{GoName: "C", Filterable: true},
		}, []string{"A", "C"}},
		{"given all filterable fields when called then returns all", []Field{
			{GoName: "A", Filterable: true},
			{GoName: "B", Filterable: true},
		}, []string{"A", "B"}},
		{"given empty entity when called then returns nil", nil, nil},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			e := Entity{Fields: tc.in}
			got := e.Filterable()
			var names []string
			for _, f := range got {
				names = append(names, f.GoName)
			}
			if !reflect.DeepEqual(names, tc.want) {
				t.Fatalf("Filterable() names = %v, want %v", names, tc.want)
			}
		})
	}
}

func TestEntity_Sortable_GivenFields_WhenCalled_ThenReturnsFieldsFlaggedSortableInOrder(t *testing.T) {
	cases := []struct {
		name string
		in   []Field
		want []string
	}{
		{"given no sortable fields when called then returns nil", []Field{{GoName: "A"}}, nil},
		{"given mixed fields when called then returns only sortable ones in order", []Field{
			{GoName: "A", Sortable: true},
			{GoName: "B"},
			{GoName: "C", Sortable: true},
		}, []string{"A", "C"}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			e := Entity{Fields: tc.in}
			got := e.Sortable()
			var names []string
			for _, f := range got {
				names = append(names, f.GoName)
			}
			if !reflect.DeepEqual(names, tc.want) {
				t.Fatalf("Sortable() names = %v, want %v", names, tc.want)
			}
		})
	}
}
