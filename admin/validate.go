package admin

import (
	"fmt"
	"reflect"
)

// Validator is implemented by any type that knows how to check its own
// invariants. The framework calls Validate() on the entity value, and on
// every exported field whose type satisfies this interface, before a create
// or update request is handed off to Storage.
type Validator interface {
	Validate() error
}

var validatorType = reflect.TypeOf((*Validator)(nil)).Elem()

func runValidators(m *entityMeta, entity reflect.Value) error {
	if err := callValidate(entity); err != nil {
		return err
	}
	for _, f := range m.fields {
		fv := entity.FieldByIndex(f.index)
		if len(f.enumOptions) > 0 {
			if err := enforceEnum(f, fv); err != nil {
				return err
			}
		}
		if err := callValidate(fv); err != nil {
			return fmt.Errorf("%s: %w", f.jsonName, err)
		}
	}
	return nil
}

func callValidate(v reflect.Value) error {
	if !v.IsValid() {
		return nil
	}
	if v.CanInterface() && v.Type().Implements(validatorType) {
		return v.Interface().(Validator).Validate()
	}
	if v.CanAddr() && v.Addr().Type().Implements(validatorType) {
		return v.Addr().Interface().(Validator).Validate()
	}
	return nil
}

func enforceEnum(f *fieldInfo, v reflect.Value) error {
	if v.Kind() != reflect.String {
		return nil
	}
	s := v.String()
	if s == "" {
		return nil
	}
	for _, opt := range f.enumOptions {
		if opt == s {
			return nil
		}
	}
	return fmt.Errorf("%s: value %q is not one of %v", f.jsonName, s, f.enumOptions)
}
