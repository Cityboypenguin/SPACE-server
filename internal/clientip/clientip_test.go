package clientip

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/labstack/echo/v4"
)

func TestExtractor(t *testing.T) {
	extract := Extractor()
	cases := []struct {
		name   string
		remote string
		xff    string
		want   string
	}{
		{"CloudFront から直接", "3.172.8.10:443", "203.0.113.5", "203.0.113.5"},
		{"CloudFront と nginx を通る", "172.18.0.1:5000", "203.0.113.5, 3.172.8.10", "203.0.113.5"},
		{"利用者が付けた偽の値は使わない", "3.172.8.10:443", "198.51.100.1, 203.0.113.5", "203.0.113.5"},
		{"CloudFront 以外からの直接の接続は接続元", "198.51.100.9:443", "203.0.113.5", "198.51.100.9"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodPost, "/query", nil)
			req.RemoteAddr = tc.remote
			req.Header.Set(echo.HeaderXForwardedFor, tc.xff)
			if got := extract(req); got != tc.want {
				t.Fatalf("got %q, want %q", got, tc.want)
			}
		})
	}
}
