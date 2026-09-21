package auth

import "context"

type contextKey string

const claimsKey contextKey = "auth_claims"

func WithClaims(ctx context.Context, claims *Claims) context.Context {
	return context.WithValue(ctx, claimsKey, claims)
}

func ClaimsFromContext(ctx context.Context) (*Claims, bool) {
	claims, ok := ctx.Value(claimsKey).(*Claims)
	return claims, ok
}

// tokenKey は claims の元になったアクセストークンそのもの。
//
// claims だけでは、後から「まだ通用するか」を確かめ直せない。失効の判定
// （RevokedTokenRepository）はトークン文字列で引くためで、claims には
// その文字列が入っていない。長寿命の接続（WebSocket / SSE）は接続中に
// 確かめ直す必要があるので、検証した側が元の文字列も残しておく。
const tokenKey contextKey = "auth_token"

// WithToken は検証済みのアクセストークンを ctx に載せる。
// 載せるのは検証した本人（ミドルウェア・WebSocket の init）だけ。
func WithToken(ctx context.Context, token string) context.Context {
	return context.WithValue(ctx, tokenKey, token)
}

// TokenFromContext は WithToken で載せたアクセストークンを返す。
func TokenFromContext(ctx context.Context) (string, bool) {
	token, ok := ctx.Value(tokenKey).(string)
	return token, ok
}
