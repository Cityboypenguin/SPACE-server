// Package clientip は、リクエストを送った利用者の IP を取り出して context に載せる。
//
// API は CloudFront を通って届くので、接続元は CloudFront の中継サーバーになる。
// 中継サーバーを信頼する経路に入れないと、速度制限やログイン失敗の回数が
// 中継サーバーごとにまとめて数えられ、無関係な利用者どうしが巻き添えになる。
package clientip

import (
	"context"
	_ "embed"
	"net"
	"strings"

	"github.com/labstack/echo/v4"
)

//go:embed cloudfront_ranges.txt
var cloudFrontRanges string

func trustCloudFront() []echo.TrustOption {
	var opts []echo.TrustOption
	for _, line := range strings.Split(cloudFrontRanges, "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		_, ipNet, err := net.ParseCIDR(line)
		if err != nil {
			panic("clientip: invalid CloudFront range " + line)
		}
		opts = append(opts, echo.TrustIPRange(ipNet))
	}
	return opts
}

// Extractor は X-Forwarded-For を右から辿り、手前の proxy（同じホストの nginx や
// docker、CloudFront）を飛ばした最初の IP を利用者の IP とする。
// 利用者が自分で付けた X-Forwarded-For は、信頼する経路より左にあるので使われない。
func Extractor() echo.IPExtractor {
	opts := append([]echo.TrustOption{echo.TrustPrivateNet(true)}, trustCloudFront()...)
	return echo.ExtractIPFromXFFHeader(opts...)
}

type contextKey struct{}

// Middleware は c.RealIP() を context に載せ、resolver や usecase から FromContext で読めるようにする。
func Middleware() echo.MiddlewareFunc {
	return func(next echo.HandlerFunc) echo.HandlerFunc {
		return func(c echo.Context) error {
			req := c.Request()
			c.SetRequest(req.WithContext(WithIP(req.Context(), c.RealIP())))
			return next(c)
		}
	}
}

// WithIP は ip を載せた context を返す。
func WithIP(ctx context.Context, ip string) context.Context {
	return context.WithValue(ctx, contextKey{}, ip)
}

// FromContext は Middleware が載せた IP を返す。載っていなければ空文字。
func FromContext(ctx context.Context) string {
	ip, _ := ctx.Value(contextKey{}).(string)
	return ip
}
