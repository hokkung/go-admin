package admin

import (
	"context"
	"fmt"
	"reflect"
	"sync"
	"sync/atomic"
)

type Storage interface {
	Create(ctx context.Context, meta StorageMeta, entity reflect.Value) (reflect.Value, error)
	Get(ctx context.Context, meta StorageMeta, id any) (reflect.Value, error)
	Update(ctx context.Context, meta StorageMeta, id any, entity reflect.Value) (reflect.Value, error)
	Delete(ctx context.Context, meta StorageMeta, id any) error
	// List executes a pre-validated query against the backing store. The
	// framework guarantees that every Filter/SortSpec references a field that
	// is filterable/sortable on this entity, and that Page/PageSize are
	// non-zero defaults, before reaching the storage.
	List(ctx context.Context, meta StorageMeta, query ListQuery) (ListResult, error)
}

type StorageMeta struct {
	Name        string
	Type        reflect.Type
	IDKind      reflect.Kind
	NewZero     func() reflect.Value
	GetID       func(reflect.Value) any
	SetID       func(reflect.Value, any) error
	FieldByJSON map[string]StorageField
}

// StorageField exposes just enough reflection info for a storage adapter to
// read a field's value or build a column reference without needing access to
// the package-private fieldInfo.
type StorageField struct {
	JSONName string
	Index    []int
	Kind     reflect.Kind
}

func metaFor(m *entityMeta) StorageMeta {
	fields := make(map[string]StorageField, len(m.fields))
	for _, f := range m.fields {
		fields[f.jsonName] = StorageField{
			JSONName: f.jsonName,
			Index:    f.index,
			Kind:     f.kind,
		}
	}
	return StorageMeta{
		Name:        m.name,
		Type:        m.typ,
		IDKind:      m.idField.kind,
		NewZero:     m.newInstance,
		GetID:       m.getID,
		SetID:       m.setID,
		FieldByJSON: fields,
	}
}

type MemoryStorage struct {
	mu    sync.RWMutex
	items map[string]reflect.Value
	seq   atomic.Int64
}

func NewMemoryStorage() *MemoryStorage {
	return &MemoryStorage{items: map[string]reflect.Value{}}
}

func (s *MemoryStorage) keyFor(id any) string {
	return fmt.Sprintf("%v", id)
}

func (s *MemoryStorage) Create(_ context.Context, meta StorageMeta, entity reflect.Value) (reflect.Value, error) {
	id := meta.GetID(entity)
	if isZero(reflect.ValueOf(id)) {
		generated, err := s.generateID(meta)
		if err != nil {
			return reflect.Value{}, err
		}
		if err := meta.SetID(entity, generated); err != nil {
			return reflect.Value{}, err
		}
		id = generated
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	key := s.keyFor(id)
	if _, exists := s.items[key]; exists {
		return reflect.Value{}, NewError(409, "ALREADY_EXISTS", fmt.Sprintf("%s with id %v already exists", meta.Name, id))
	}
	s.items[key] = entity
	return entity, nil
}

func (s *MemoryStorage) Get(_ context.Context, meta StorageMeta, id any) (reflect.Value, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	v, ok := s.items[s.keyFor(id)]
	if !ok {
		return reflect.Value{}, ErrNotFound
	}
	return v, nil
}

func (s *MemoryStorage) Update(_ context.Context, meta StorageMeta, id any, entity reflect.Value) (reflect.Value, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	key := s.keyFor(id)
	if _, ok := s.items[key]; !ok {
		return reflect.Value{}, ErrNotFound
	}
	if err := meta.SetID(entity, id); err != nil {
		return reflect.Value{}, err
	}
	s.items[key] = entity
	return entity, nil
}

func (s *MemoryStorage) Delete(_ context.Context, _ StorageMeta, id any) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	key := s.keyFor(id)
	if _, ok := s.items[key]; !ok {
		return ErrNotFound
	}
	delete(s.items, key)
	return nil
}

func (s *MemoryStorage) List(_ context.Context, meta StorageMeta, q ListQuery) (ListResult, error) {
	s.mu.RLock()
	items := make([]reflect.Value, 0, len(s.items))
	for _, v := range s.items {
		items = append(items, v)
	}
	s.mu.RUnlock()
	return ApplyInMemoryQuery(meta, items, q)
}

func (s *MemoryStorage) generateID(meta StorageMeta) (any, error) {
	switch meta.IDKind {
	case reflect.String:
		return fmt.Sprintf("%s_%d", meta.Name, s.seq.Add(1)), nil
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		return s.seq.Add(1), nil
	case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64:
		return uint64(s.seq.Add(1)), nil
	}
	return nil, fmt.Errorf("cannot auto-generate id of kind %s", meta.IDKind)
}

func isZero(v reflect.Value) bool {
	if !v.IsValid() {
		return true
	}
	return v.IsZero()
}
