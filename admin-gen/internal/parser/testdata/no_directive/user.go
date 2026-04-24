package no_directive

// User has no //admin:generate, so Parse should skip it and return an
// empty slice.
type User struct {
	ID   uint
	Name string
}
