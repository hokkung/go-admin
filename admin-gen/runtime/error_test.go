package runtime

import (
	"encoding/json"
	"errors"
	"io"
	"net/http/httptest"
	"testing"

	"github.com/gofiber/fiber/v2"
)

func TestNewError_GivenStatusCodeAndMessage_WhenConstructed_ThenAllFieldsPopulated(t *testing.T) {
	e := NewError(418, "TEAPOT", "short and stout")
	if e.Status != 418 || e.Code != "TEAPOT" || e.Message != "short and stout" {
		t.Fatalf("NewError fields = %+v", e)
	}
	if e.Error() != "short and stout" {
		t.Fatalf("Error() = %q, want message", e.Error())
	}
}

func TestBadRequest_GivenMessage_WhenCalled_ThenReturns400WithBadRequestCode(t *testing.T) {
	e := BadRequest("bad things happened")
	if e.Status != 400 || e.Code != "BAD_REQUEST" {
		t.Fatalf("BadRequest shape = %+v", e)
	}
	if e.Message != "bad things happened" {
		t.Fatalf("BadRequest.Message = %q", e.Message)
	}
}

func TestValidationError_GivenWrappedError_WhenCalled_ThenReturns400WithValidationFailedCode(t *testing.T) {
	e := ValidationError(errors.New("email: invalid"))
	if e.Status != 400 || e.Code != "VALIDATION_FAILED" {
		t.Fatalf("ValidationError shape = %+v", e)
	}
	if e.Message != "email: invalid" {
		t.Fatalf("ValidationError.Message = %q", e.Message)
	}
}

func TestNotImplemented_GivenActionName_WhenCalled_ThenReturns501WithActionInMessage(t *testing.T) {
	e := NotImplemented("activate")
	if e.Status != 501 || e.Code != "NOT_IMPLEMENTED" {
		t.Fatalf("NotImplemented shape = %+v", e)
	}
	if e.Message != "action activate has no handler registered" {
		t.Fatalf("NotImplemented.Message = %q", e.Message)
	}
}

// WriteError is exercised via a throwaway fiber App rather than a hand-rolled
// fiber.Ctx — constructing a Ctx directly isn't part of fiber's public API.
func TestWriteError_GivenVariousErrorKinds_WhenWritten_ThenStatusAndEnvelopeMatchError(t *testing.T) {
	cases := []struct {
		name       string // "given X when Y then Z"
		err        error
		wantStatus int
		wantCode   string
	}{
		{"given typed Error when written then uses its Status and Code", NewError(404, "NOT_FOUND", "gone"), 404, "NOT_FOUND"},
		{"given plain error when written then coerces to 500 INTERNAL", errors.New("boom"), 500, "INTERNAL"},
		{"given BadRequest helper when written then emits 400 BAD_REQUEST", BadRequest("nope"), 400, "BAD_REQUEST"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			app := fiber.New(fiber.Config{DisableStartupMessage: true})
			app.Get("/boom", func(c *fiber.Ctx) error { return WriteError(c, tc.err) })

			resp, err := app.Test(httptest.NewRequest("GET", "/boom", nil))
			if err != nil {
				t.Fatalf("app.Test: %v", err)
			}
			if resp.StatusCode != tc.wantStatus {
				t.Fatalf("status = %d, want %d", resp.StatusCode, tc.wantStatus)
			}
			body, _ := io.ReadAll(resp.Body)
			var env struct {
				Error struct {
					Code    string `json:"code"`
					Message string `json:"message"`
				} `json:"error"`
			}
			if err := json.Unmarshal(body, &env); err != nil {
				t.Fatalf("decode: %v (body=%s)", err, body)
			}
			if env.Error.Code != tc.wantCode {
				t.Fatalf("code = %q, want %q", env.Error.Code, tc.wantCode)
			}
			if env.Error.Message == "" {
				t.Fatalf("message is empty; body=%s", body)
			}
		})
	}
}
