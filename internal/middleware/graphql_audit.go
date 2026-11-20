package middleware

import (
	"bytes"
	"encoding/json"
	"io"
	"regexp"
	"time"

	"github.com/Cityboypenguin/SPACE-server/internal/audit"
	"github.com/Cityboypenguin/SPACE-server/internal/logger"
	"github.com/labstack/echo/v4"
)

type graphqlRequestPayload struct {
	OperationName string         `json:"operationName"`
	Query         string         `json:"query"`
	Variables     map[string]any `json:"variables"`
}

const maskedValue = "***"

// sensitivePattern はログ出力時にマスクするフィールド名（大文字小文字・部分一致）。
// otp はメール認証やパスワード再設定のワンタイムパスワード。
// ここに当たる項目は、値が文字列でなくても丸ごと伏せる。
var sensitivePattern = regexp.MustCompile(`(?i)password|passwd|secret|token|otp|passcode|credential`)

// idKeyPattern は値を残してよい ID の項目名（id、roomID、user_id、mentionUserIDs など）。
var idKeyPattern = regexp.MustCompile(`^ids?$|_ids?$|I[Dd]s?$`)

// keptStringKeys は ID 以外で、文字列の値を残してよい項目名。調査に要り、
// 利用者が書いた文章を含まないものだけを並べる。
//
// ここに無い文字列の項目はすべて伏せる。DM・投稿の本文（content）、自己紹介（bio）、
// 表示名（name）、検索語（keyword、tag）などが平文でログに残っていたため、
// 「伏せる項目を並べる」のではなく「残す項目を並べる」形にしている。
// 項目を足すときは、利用者の書いた文章が入らないことを確かめてから足すこと。
var keptStringKeys = map[string]bool{
	"after": true, "before": true, "since": true, "from": true, "to": true,
	"granularity": true, "semester": true,
	"status": true, "type": true, "sortBy": true, "sortOrder": true,
	"contentType": true, "objectKey": true, "emoji": true,
	"path":  true, // 閲覧記録の画面のパス
	"email": true, // ログインできない問い合わせの調査に使う
}

// maskVariables は入れ子の input オブジェクトや配列の中まで見てマスクする。
// ログインや登録はパスワードを variables.input の下に入れて送ってくるため。
// 数値・真偽値・null はそのまま残し、文字列は項目名で残すかを決める。
func maskVariables(vars map[string]any) map[string]any {
	if len(vars) == 0 {
		return vars
	}
	masked := make(map[string]any, len(vars))
	for k, v := range vars {
		if sensitivePattern.MatchString(k) {
			masked[k] = maskedValue
		} else {
			masked[k] = maskValue(k, v)
		}
	}
	return masked
}

// maskValue は key の値 v を伏せる。配列の要素は親の項目名で判断する。
func maskValue(key string, v any) any {
	switch t := v.(type) {
	case map[string]any:
		return maskVariables(t)
	case []any:
		out := make([]any, len(t))
		for i, item := range t {
			out[i] = maskValue(key, item)
		}
		return out
	case string:
		if keptStringKeys[key] || idKeyPattern.MatchString(key) {
			return t
		}
		return maskedValue
	default:
		return v
	}
}

func parseGraphQLBody(c echo.Context) (operationName, maskedVars string) {
	bodyBytes, err := io.ReadAll(c.Request().Body)
	c.Request().Body = io.NopCloser(bytes.NewBuffer(bodyBytes))
	if err != nil {
		logger.Log.Warn().Err(err).Msg("graphql_audit: failed to read request body")
		return
	}
	if len(bodyBytes) == 0 {
		return
	}
	var payload graphqlRequestPayload
	if err := json.Unmarshal(bodyBytes, &payload); err != nil {
		return
	}
	operationName = payload.OperationName
	if len(payload.Variables) == 0 {
		return
	}
	b, err := json.Marshal(maskVariables(payload.Variables))
	if err != nil {
		return
	}
	maskedVars = string(b)
	return
}

func GraphQLAudit() echo.MiddlewareFunc {
	return func(next echo.HandlerFunc) echo.HandlerFunc {
		return func(c echo.Context) error {
			if c.Path() != "/query" {
				return next(c)
			}

			start := time.Now()
			opName := c.QueryParam("operationName")

			var maskedVars string
			if c.Request().Method == echo.POST {
				parsedOp, parsedVars := parseGraphQLBody(c)
				if parsedOp != "" {
					opName = parsedOp
				}
				maskedVars = parsedVars
			}

			err := next(c)
			status := c.Response().Status
			duration := time.Since(start).Milliseconds()

			audit.WithActor(logger.Log.Info().
				Str("event", "graphql_audit").
				Str("op", opName).
				Int("status", status).
				Str("ip", c.RealIP()).
				Int64("duration_ms", duration).
				Str("vars", maskedVars), c.Request().Context()).Send()

			return err
		}
	}
}
