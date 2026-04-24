package runtime

import (
	"errors"
	"strings"
	"testing"
)

// --- EnforceEnum --------------------------------------------------------

func TestEnforceEnum_GivenValueAndOptions_WhenEnforced_ThenAcceptsMembersAndRejectsOthers(t *testing.T) {
	opts := []string{"active", "inactive"}
	cases := []struct {
		name    string
		value   string
		options []string
		wantErr string // empty = no error; otherwise a substring of the message
	}{
		{"given empty value when enforced then accepts as unset", "", opts, ""},
		{"given value equal to first option when enforced then returns nil", "active", opts, ""},
		{"given value equal to last option when enforced then returns nil", "inactive", opts, ""},
		{"given value not in options when enforced then returns prefixed error", "draft", opts, `"draft" is not one of`},
		{"given empty options and non-empty value when enforced then returns prefixed error", "x", []string{}, `"x" is not one of`},
		{"given nil options and non-empty value when enforced then returns prefixed error", "x", nil, `"x" is not one of`},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			err := EnforceEnum("status", tc.value, tc.options)
			switch {
			case tc.wantErr == "" && err != nil:
				t.Fatalf("wanted nil err, got %v", err)
			case tc.wantErr != "" && err == nil:
				t.Fatalf("wanted err containing %q, got nil", tc.wantErr)
			case err != nil && !strings.Contains(err.Error(), tc.wantErr):
				t.Fatalf("err = %v, want substring %q", err, tc.wantErr)
			case err != nil && !strings.HasPrefix(err.Error(), "status:"):
				t.Fatalf("err = %q, want JSON-name prefix", err)
			}
		})
	}
}

// --- CallFieldValidator -------------------------------------------------

// valueValidator satisfies Validator on the VALUE receiver. Both `value` and
// `&value` forms will satisfy the interface assertion at runtime — the helper
// should stop after the first successful dispatch so Validate fires exactly
// once per call.
type valueValidator struct {
	name string
	err  error
	// count is incremented each time Validate runs. Tests verify it reaches
	// at most 1 per CallFieldValidator invocation.
	count *int
}

func (v valueValidator) Validate() error {
	if v.count != nil {
		*v.count++
	}
	return v.err
}

// pointerValidator only implements Validator on *pointerValidator. Passing
// the value form should not dispatch; only the pointer form should.
type pointerValidator struct {
	err   error
	count *int
}

func (p *pointerValidator) Validate() error {
	if p.count != nil {
		*p.count++
	}
	return p.err
}

// plain has no Validate method at all.
type plain struct{}

func TestCallFieldValidator_GivenTypeWithoutValidate_WhenCalled_ThenReturnsNil(t *testing.T) {
	x := plain{}
	if err := CallFieldValidator("x", x, &x); err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
}

func TestCallFieldValidator_GivenValueReceiverValidator_WhenCalled_ThenFiresOnceAndReturnsNil(t *testing.T) {
	n := 0
	v := valueValidator{count: &n}
	if err := CallFieldValidator("email", v, &v); err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	// Both value and &value satisfy Validator when the method is on the
	// value receiver; the helper must return after the value dispatch so
	// Validate doesn't double-fire.
	if n != 1 {
		t.Fatalf("Validate fired %d times, want 1", n)
	}
}

func TestCallFieldValidator_GivenPointerReceiverValidator_WhenCalled_ThenFiresOnceAndReturnsNil(t *testing.T) {
	n := 0
	p := pointerValidator{count: &n}
	if err := CallFieldValidator("email", p, &p); err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	if n != 1 {
		t.Fatalf("Validate fired %d times, want 1 (pointer receiver)", n)
	}
}

func TestCallFieldValidator_GivenValueValidatorReturningError_WhenCalled_ThenPrefixesErrorWithJSONName(t *testing.T) {
	v := valueValidator{err: errors.New("invalid format")}
	err := CallFieldValidator("email_address", v, &v)
	if err == nil || err.Error() != "email_address: invalid format" {
		t.Fatalf("err = %v, want \"email_address: invalid format\"", err)
	}
}

func TestCallFieldValidator_GivenPointerValidatorReturningError_WhenCalled_ThenPrefixesErrorWithJSONName(t *testing.T) {
	p := pointerValidator{err: errors.New("bad")}
	err := CallFieldValidator("name", p, &p)
	if err == nil || err.Error() != "name: bad" {
		t.Fatalf("err = %v, want \"name: bad\"", err)
	}
}

// --- CallEntityValidator ------------------------------------------------

func TestCallEntityValidator_GivenTypeWithoutValidate_WhenCalled_ThenReturnsNil(t *testing.T) {
	x := plain{}
	if err := CallEntityValidator(x, &x); err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
}

func TestCallEntityValidator_GivenValueReceiverValidator_WhenCalled_ThenFiresOnce(t *testing.T) {
	n := 0
	v := valueValidator{count: &n}
	if err := CallEntityValidator(v, &v); err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	if n != 1 {
		t.Fatalf("Validate fired %d times, want 1", n)
	}
}

func TestCallEntityValidator_GivenPointerReceiverValidator_WhenCalled_ThenFiresOnce(t *testing.T) {
	n := 0
	p := pointerValidator{count: &n}
	if err := CallEntityValidator(p, &p); err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	if n != 1 {
		t.Fatalf("Validate fired %d times, want 1 (pointer receiver)", n)
	}
}

func TestCallEntityValidator_GivenValidatorReturningError_WhenCalled_ThenReturnsErrorVerbatimWithoutPrefix(t *testing.T) {
	// Entity-level errors do NOT get a prefix — the handler wraps them as
	// VALIDATION_FAILED, so "User: bad" would just be noise on the wire.
	v := valueValidator{err: errors.New("whole thing is wrong")}
	err := CallEntityValidator(v, &v)
	if err == nil || err.Error() != "whole thing is wrong" {
		t.Fatalf("err = %v, want verbatim", err)
	}
}
