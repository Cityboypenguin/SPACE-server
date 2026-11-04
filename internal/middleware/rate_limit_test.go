package middleware

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/Cityboypenguin/SPACE-server/internal/auth"
	"github.com/labstack/echo/v4"
)

func rateLimitServer(mw echo.MiddlewareFunc) *echo.Echo {
	e := echo.New()
	e.Use(func(next echo.HandlerFunc) echo.HandlerFunc {
		return func(c echo.Context) error {
			if id := c.Request().Header.Get("X-Test-User"); id != "" {
				ctx := auth.WithClaims(c.Request().Context(), &auth.Claims{ID: int64(len(id)), Role: "user"})
				c.SetRequest(c.Request().WithContext(ctx))
			}
			return next(c)
		}
	})
	e.Use(mw)
	e.POST("/query", func(c echo.Context) error { return c.NoContent(http.StatusOK) })
	return e
}

func hit(e *echo.Echo, user string) int {
	req := httptest.NewRequest(http.MethodPost, "/query", nil)
	req.RemoteAddr = "203.0.113.1:1000"
	if user != "" {
		req.Header.Set("X-Test-User", user)
	}
	rec := httptest.NewRecorder()
	e.ServeHTTP(rec, req)
	return rec.Code
}

func TestGraphQLRateLimit_CountsEachUserSeparatelyOnASharedIP(t *testing.T) {
	e := rateLimitServer(GraphQLRateLimit(nil))
	limited := false
	for range int(GraphQLRateLimitBurst) * 3 {
		if hit(e, "a") == http.StatusTooManyRequests {
			limited = true
			break
		}
	}
	if !limited {
		t.Fatal("a user over the limit should be rejected")
	}
	if code := hit(e, "bb"); code != http.StatusOK {
		t.Errorf("another user on the same IP: code = %d", code)
	}
	if code := hit(e, ""); code != http.StatusOK {
		t.Errorf("not logged in on the same IP: code = %d", code)
	}
}

func TestGraphQLRateLimit_CountsAnonymousRequestsByIP(t *testing.T) {
	e := rateLimitServer(GraphQLRateLimit(nil))
	for range int(GraphQLRateLimitBurst) * 3 {
		if hit(e, "") == http.StatusTooManyRequests {
			return
		}
	}
	t.Fatal("anonymous requests over the limit should be rejected")
}

func TestGraphQLIPRateLimit_AllowsMoreThanOneUsersLimit(t *testing.T) {
	e := rateLimitServer(GraphQLIPRateLimit(nil))
	for i := range int(GraphQLRateLimitBurst) * 2 {
		if code := hit(e, "a"); code != http.StatusOK {
			t.Fatalf("request %d: code = %d", i, code)
		}
	}
}
