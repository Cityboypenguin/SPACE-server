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

// sensitivePattern はログ出力時にマスクするフィールド名（大文字小文字・部分一致）。
// otp はメール認証やパスワード再設定のワンタイムパスワード。
var sensitivePattern = regexp.MustCompile(`(?i)password|passwd|secret|token|otp|passcode|credential`)

// maskVariables は入れ子の input オブジェクトや配列の中まで見てマスクする。
// ログインや登録はパスワードを variables.input の下に入れて送ってくるため。
func maskVariables(vars map[string]any) map[string]any {
	if len(vars) == 0 {
		return vars
	}
	masked := make(map[string]any, len(vars))
	for k, v := range vars {
		if sensitivePattern.MatchString(k) {
			masked[k] = "***"
		} else {
			masked[k] = maskValue(v)
		}
	}
	return masked
}

func maskValue(v any) any {
	switch t := v.(type) {
	case map[string]any:
		return maskVariables(t)
	case []any:
		out := make([]any, len(t))
		for i, item := range t {
			out[i] = maskValue(item)
		}
		return out
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
