package runtime

import "fmt"

// Validator is the optional interface generated handlers look for before
// persisting a create or update. If your entity (or any of its field types)
// satisfies it, .Validate() is called and a non-nil return becomes a
// VALIDATION_FAILED response.
//
// Nothing here is magic — the generated handler does a plain type assertion,
// which you can see and edit in <entity>_handlers.go.
type Validator interface {
	Validate() error
}

// EnforceEnum returns a prefixed error if value is a non-empty string that
// isn't in options. Empty values are allowed — matches the runtime admin
// package, which treats the zero string as "unset" rather than "invalid".
func EnforceEnum(jsonName, value string, options []string) error {
	if value == "" {
		return nil
	}
	for _, opt := range options {
		if opt == value {
			return nil
		}
	}
	return fmt.Errorf("%s: value %q is not one of %v", jsonName, value, options)
}

// CallFieldValidator tries Validate() on value, then on ptr, and returns the
// first non-nil error with jsonName prepended (e.g. "email: invalid format").
// Generated validate<Entity> functions call this once per field. Stops after
// the first successful dispatch so side effects aren't doubled when both the
// value and pointer forms satisfy the interface.
func CallFieldValidator(jsonName string, value, ptr any) error {
	if v, ok := value.(Validator); ok {
		if err := v.Validate(); err != nil {
			return fmt.Errorf("%s: %w", jsonName, err)
		}
		return nil
	}
	if v, ok := ptr.(Validator); ok {
		if err := v.Validate(); err != nil {
			return fmt.Errorf("%s: %w", jsonName, err)
		}
	}
	return nil
}

// CallEntityValidator is the non-prefixed version for the entity-level
// Validate(). Returning the raw error lets the handler wrap it once as
// VALIDATION_FAILED — adding a prefix here would make the wire message
// "<Entity>: <msg>" which clients do not need.
func CallEntityValidator(value, ptr any) error {
	if v, ok := value.(Validator); ok {
		return v.Validate()
	}
	if v, ok := ptr.(Validator); ok {
		return v.Validate()
	}
	return nil
}
