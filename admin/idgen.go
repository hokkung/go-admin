package admin

import (
	"fmt"
	"reflect"
	"sync/atomic"

	"github.com/google/uuid"
)

// IDGenerator produces an id value for a new entity when the client did not
// supply one. The returned value is written into the entity's id field via
// reflection, so its type must be assignable or convertible to the field type.
type IDGenerator func(meta StorageMeta) (any, error)

// AutoIncrement returns a generator that hands out a monotonically increasing
// integer (or "<name>_<n>" for string-kinded ids) per registered entity.
//
// Intended for in-memory or test setups. For SQL-backed storage prefer the
// database's own auto-increment so counters survive restarts.
func AutoIncrement() IDGenerator {
	var seq atomic.Int64
	return func(meta StorageMeta) (any, error) {
		n := seq.Add(1)
		switch meta.IDKind {
		case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
			return n, nil
		case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64:
			return uint64(n), nil
		case reflect.String:
			return fmt.Sprintf("%s_%d", meta.Name, n), nil
		}
		return nil, fmt.Errorf("AutoIncrement: unsupported id kind %s", meta.IDKind)
	}
}

// AutoUUID returns a generator that produces a fresh uuid.UUID for each call.
// The entity's id field must be uuid.UUID (or a named type convertible from it).
func AutoUUID() IDGenerator {
	return func(meta StorageMeta) (any, error) {
		return uuid.New(), nil
	}
}
