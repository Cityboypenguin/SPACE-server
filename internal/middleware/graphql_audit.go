package middleware

import (
	"bytes"
	"encoding/json"
	"io"
	"regexp"
	"time"

	"github.com/Cityboypenguin/SPACE-server/internal/auth"
	"github.com/Cityboypenguin/SPACE-server/internal/logger"
	"github.com/labstack/echo/v4"
)

type graphqlRequestPayload struct {
	OperationName string                 `json:"operationName"`
	Query         string                 `json:"query"`
	Variables     map[string]interface{} `json:"variables"`
}

// sensitiveKeys はログ出力時にマスクするフィールド名（大文字小文字・部分一致）。
var sensitivePattern = regexp.MustCompile(`(?i)password|secret|token`)

func maskVariables(vars map[string]interface{}) map[string]interface{} {
	if len(vars) == 0 {
		return vars
	}
	masked := make(map[string]interface{}, len(vars))
	for k, v := range vars {
		if sensitivePattern.MatchString(k) {
			masked[k] = "***"
		} else {
			masked[k] = v
		}
	}
	return masked
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
				bodyBytes, _ := io.ReadAll(c.Request().Body)
				c.Request().Body = io.NopCloser(bytes.NewBuffer(bodyBytes))

				if len(bodyBytes) > 0 {
					var payload graphqlRequestPayload
					if err := json.Unmarshal(bodyBytes, &payload); err == nil {
						if payload.OperationName != "" {
							opName = payload.OperationName
						}
						if len(payload.Variables) > 0 {
							if b, err := json.Marshal(maskVariables(payload.Variables)); err == nil {
								maskedVars = string(b)
							}
						}
					}
				}
			}

			err := next(c)
			status := c.Response().Status
			duration := time.Since(start).Milliseconds()

			ev := logger.Log.Info().
				Str("event", "graphql_audit").
				Str("op", opName).
				Int("status", status).
				Str("ip", c.RealIP()).
				Int64("duration_ms", duration).
				Str("vars", maskedVars)

			if claims, ok := auth.ClaimsFromContext(c.Request().Context()); ok {
				ev.Int64("actor_id", claims.ID).Str("actor_role", claims.Role)
			} else {
				ev.Str("actor_id", "anonymous").Str("actor_role", "anonymous")
			}
			ev.Send()

			return err
		}
	}
}
