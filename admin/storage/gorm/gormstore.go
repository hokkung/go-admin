// Package gormstore provides a GORM-backed implementation of admin.Storage.
//
// List pushes filter/sort/pagination down into SQL via Where/Order/Limit/
// Offset, plus a separate COUNT(*) for the total. Column names are taken
// from the field's JSON tag, so the JSON wire name must match the database
// column. If your schema diverges (e.g. `gorm:"column:display_name"` on a
// field tagged `json:"name"`), filtering on that field will produce invalid
// SQL — either keep the names aligned or plug in a custom Storage.
package gormstore

import (
	"context"
	"errors"
	"fmt"
	"reflect"
	"strings"

	"github.com/hokkung/go-admin/admin"
	"gorm.io/gorm"
)

type Storage struct {
	db *gorm.DB
}

func New(db *gorm.DB) *Storage {
	return &Storage{db: db}
}

func (s *Storage) Create(ctx context.Context, meta admin.StorageMeta, entity reflect.Value) (reflect.Value, error) {
	ptr := reflect.New(meta.Type)
	ptr.Elem().Set(entity)
	if err := s.db.WithContext(ctx).Create(ptr.Interface()).Error; err != nil {
		return reflect.Value{}, err
	}
	return ptr.Elem(), nil
}

func (s *Storage) Get(ctx context.Context, meta admin.StorageMeta, id any) (reflect.Value, error) {
	ptr := reflect.New(meta.Type)
	if err := s.db.WithContext(ctx).First(ptr.Interface(), id).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return reflect.Value{}, admin.ErrNotFound
		}
		return reflect.Value{}, err
	}
	return ptr.Elem(), nil
}

func (s *Storage) Update(ctx context.Context, meta admin.StorageMeta, id any, entity reflect.Value) (reflect.Value, error) {
	if err := meta.SetID(entity, id); err != nil {
		return reflect.Value{}, err
	}
	ptr := reflect.New(meta.Type)
	ptr.Elem().Set(entity)
	if err := s.db.WithContext(ctx).Save(ptr.Interface()).Error; err != nil {
		return reflect.Value{}, err
	}
	return ptr.Elem(), nil
}

func (s *Storage) Delete(ctx context.Context, meta admin.StorageMeta, id any) error {
	ptr := reflect.New(meta.Type)
	tx := s.db.WithContext(ctx).Delete(ptr.Interface(), id)
	if tx.Error != nil {
		return tx.Error
	}
	if tx.RowsAffected == 0 {
		return admin.ErrNotFound
	}
	return nil
}

func (s *Storage) List(ctx context.Context, meta admin.StorageMeta, q admin.ListQuery) (admin.ListResult, error) {
	slicePtr := reflect.New(reflect.SliceOf(meta.Type))

	tx := s.db.WithContext(ctx).Model(slicePtr.Interface())
	for _, f := range q.Filters {
		expr, args, err := filterToSQL(f)
		if err != nil {
			return admin.ListResult{}, err
		}
		tx = tx.Where(expr, args...)
	}

	// Count the filtered total before applying pagination. Session clones the
	// current statement so Count doesn't consume the chained Where clauses.
	var total int64
	if err := tx.Session(&gorm.Session{}).Count(&total).Error; err != nil {
		return admin.ListResult{}, err
	}

	for _, sp := range q.Sort {
		dir := "ASC"
		if strings.EqualFold(sp.Order, "desc") {
			dir = "DESC"
		}
		tx = tx.Order(quoteIdent(sp.Field) + " " + dir)
	}

	offset := (q.Page - 1) * q.PageSize
	if err := tx.Offset(offset).Limit(q.PageSize).Find(slicePtr.Interface()).Error; err != nil {
		return admin.ListResult{}, err
	}

	slice := slicePtr.Elem()
	items := make([]any, slice.Len())
	for i := 0; i < slice.Len(); i++ {
		items[i] = slice.Index(i).Interface()
	}
	return admin.ListResult{
		Items:    items,
		Total:    int(total),
		Page:     q.Page,
		PageSize: q.PageSize,
	}, nil
}

func filterToSQL(f admin.Filter) (string, []any, error) {
	col := quoteIdent(f.Field)
	switch f.Op {
	case "", "eq":
		return col + " = ?", []any{f.Value}, nil
	case "ne":
		return col + " <> ?", []any{f.Value}, nil
	case "lt":
		return col + " < ?", []any{f.Value}, nil
	case "lte":
		return col + " <= ?", []any{f.Value}, nil
	case "gt":
		return col + " > ?", []any{f.Value}, nil
	case "gte":
		return col + " >= ?", []any{f.Value}, nil
	case "contains":
		s, ok := f.Value.(string)
		if !ok {
			return "", nil, fmt.Errorf("contains op requires string value for field %q", f.Field)
		}
		return col + " LIKE ?", []any{"%" + s + "%"}, nil
	case "in":
		return col + " IN ?", []any{f.Value}, nil
	}
	return "", nil, fmt.Errorf("unknown operator %q", f.Op)
}

// quoteIdent wraps a column name in double quotes to be safe against
// reserved words (works on Postgres and SQLite, which both accept
// double-quoted identifiers). The caller is responsible for ensuring the
// name came from the admin framework's validated field list — the framework
// already rejects anything that's not a registered filterable/sortable
// field, so this is an allowlist, not a sanitisation.
func quoteIdent(name string) string {
	return `"` + strings.ReplaceAll(name, `"`, `""`) + `"`
}
