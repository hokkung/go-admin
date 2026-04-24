package runtime

import (
	"strings"
	"testing"
)

func TestNormalizeQuery_GivenVariousInputs_WhenNormalized_ThenDefaultsAndClampsApplied(t *testing.T) {
	cases := []struct {
		name            string
		in              ListQuery
		defaultPageSize int
		maxPageSize     int
		wantPage        int
		wantPageSize    int
	}{
		{"given zero values when normalized then applies defaults", ListQuery{}, 20, 100, 1, 20},
		{"given negative page when normalized then resets page to 1", ListQuery{Page: -3, PageSize: 10}, 20, 100, 1, 10},
		{"given page_size above max when normalized then clamps to max", ListQuery{PageSize: 500}, 20, 100, 1, 100},
		{"given maxPageSize zero when normalized then leaves page_size untouched", ListQuery{PageSize: 10000}, 20, 0, 1, 10000},
		{"given explicit values when normalized then preserves them", ListQuery{Page: 3, PageSize: 25}, 20, 100, 3, 25},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			q := tc.in
			NormalizeQuery(&q, tc.defaultPageSize, tc.maxPageSize)
			if q.Page != tc.wantPage || q.PageSize != tc.wantPageSize {
				t.Fatalf("got {Page:%d PageSize:%d}, want {Page:%d PageSize:%d}", q.Page, q.PageSize, tc.wantPage, tc.wantPageSize)
			}
		})
	}
}

func TestQuoteIdent_GivenIdentifier_WhenQuoted_ThenWrapsInDoubleQuotesAndEscapesEmbeddedOnes(t *testing.T) {
	cases := []struct {
		name     string
		in, want string
	}{
		{"given plain name when quoted then surrounded by double quotes", "name", `"name"`},
		{"given empty string when quoted then returns empty pair", "", `""`},
		{"given embedded double quote when quoted then doubles it", `with"quote`, `"with""quote"`},
		{"given already-quoted input when quoted then doubles all embedded quotes", `"already"`, `"""already"""`},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := quoteIdent(tc.in)
			if got != tc.want {
				t.Fatalf("quoteIdent(%q) = %q, want %q", tc.in, got, tc.want)
			}
		})
	}
}

// ApplyGormFilter is exercised against an offline *gorm.DB session so we can
// inspect the generated SQL without needing a real database.
func TestApplyGormFilter_GivenSupportedOperator_WhenApplied_ThenProducesExpectedWhereClause(t *testing.T) {
	cases := []struct {
		name     string
		filter   Filter
		wantFrag string // substring of the generated WHERE clause
	}{
		{"given empty op when applied then defaults to equality", Filter{Field: "name", Op: "", Value: "alice"}, `"name" = ?`},
		{"given eq op when applied then emits = clause", Filter{Field: "name", Op: "eq", Value: "alice"}, `"name" = ?`},
		{"given ne op when applied then emits <> clause", Filter{Field: "name", Op: "ne", Value: "bob"}, `"name" <> ?`},
		{"given lt op when applied then emits < clause", Filter{Field: "age", Op: "lt", Value: 18}, `"age" < ?`},
		{"given lte op when applied then emits <= clause", Filter{Field: "age", Op: "lte", Value: 65}, `"age" <= ?`},
		{"given gt op when applied then emits > clause", Filter{Field: "age", Op: "gt", Value: 21}, `"age" > ?`},
		{"given gte op when applied then emits >= clause", Filter{Field: "age", Op: "gte", Value: 0}, `"age" >= ?`},
		{"given contains op when applied then emits LIKE with %value% binding", Filter{Field: "name", Op: "contains", Value: "li"}, `"name" LIKE ?`},
		{"given in op when applied then emits IN clause", Filter{Field: "id", Op: "in", Value: []int{1, 2, 3}}, `"id" IN`},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			stmt := newTestStmt(t)
			tx := ApplyGormFilter(stmt, tc.filter.Field, tc.filter)
			sql := explainTestStmt(tx)
			if !strings.Contains(sql, tc.wantFrag) {
				t.Fatalf("sql = %q, want fragment %q", sql, tc.wantFrag)
			}
		})
	}
}

func TestApplyGormFilter_GivenUnknownOperator_WhenApplied_ThenReturnsTxUnchanged(t *testing.T) {
	stmt := newTestStmt(t)
	out := ApplyGormFilter(stmt, "name", Filter{Op: "mystery", Value: "x"})
	if out != stmt {
		t.Fatalf("unknown op should return tx unchanged")
	}
}
