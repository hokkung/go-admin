// Package gormstore provides a GORM-backed implementation of admin.Storage.
//
// Filtering/sorting/pagination are applied in-memory by the admin package
// after List returns all rows — fine for small tables, not a substitute for
// pushing predicates into SQL for large datasets.
package gormstore

import (
	"context"
	"errors"
	"reflect"

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

func (s *Storage) List(ctx context.Context, meta admin.StorageMeta) ([]reflect.Value, error) {
	slicePtr := reflect.New(reflect.SliceOf(meta.Type))
	if err := s.db.WithContext(ctx).Find(slicePtr.Interface()).Error; err != nil {
		return nil, err
	}
	slice := slicePtr.Elem()
	out := make([]reflect.Value, slice.Len())
	for i := 0; i < slice.Len(); i++ {
		out[i] = slice.Index(i)
	}
	return out, nil
}
