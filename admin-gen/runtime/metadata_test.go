package runtime

import (
	"net/http/httptest"
	"testing"

	"github.com/gofiber/fiber/v2"
)

// TestRequestBase asserts that the base prefix is the scheme+host plus the
// path up to and including the final slash. That's the value the generated
// metadata + admin.entities handlers concatenate with "<entity>.<action>" to
// produce absolute URLs.
func TestRequestBase_GivenVariousRequestPaths_WhenCalled_ThenReturnsBaseURLUpToLastSlash(t *testing.T) {
	cases := []struct {
		name      string
		requestAt string
		want      string
	}{
		{
			name:      "given request at root dispatch when called then returns scheme+host with trailing slash",
			requestAt: "/user.metadata",
			want:      "http://example.com/",
		},
		{
			name:      "given request under /admin group when called then returns base with /admin/ prefix",
			requestAt: "/admin/user.metadata",
			want:      "http://example.com/admin/",
		},
		{
			name:      "given deeply grouped request when called then preserves full prefix",
			requestAt: "/api/v1/admin/user.list",
			want:      "http://example.com/api/v1/admin/",
		},
		{
			name:      "given path already ending in slash when called then returns base unchanged",
			requestAt: "/admin/",
			want:      "http://example.com/admin/",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			app := fiber.New(fiber.Config{DisableStartupMessage: true})
			var got string
			app.All("/*", func(c *fiber.Ctx) error {
				got = RequestBase(c)
				return c.SendStatus(200)
			})
			req := httptest.NewRequest("POST", tc.requestAt, nil)
			req.Host = "example.com"
			if _, err := app.Test(req); err != nil {
				t.Fatalf("app.Test: %v", err)
			}
			if got != tc.want {
				t.Fatalf("RequestBase = %q, want %q", got, tc.want)
			}
		})
	}
}
