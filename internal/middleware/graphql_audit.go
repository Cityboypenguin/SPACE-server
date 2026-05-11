package middleware

import (
	"bytes"
	"encoding/json"
	"io"
	"log"
	"regexp"
	"time"

	"github.com/Cityboypenguin/SPACE-server/internal/auth"
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

			claims, ok := auth.ClaimsFromContext(c.Request().Context())
			if ok {
				log.Printf("audit graphql op=%s status=%d actor_id=%d actor_role=%s ip=%s duration_ms=%d vars=%s", opName, status, claims.ID, claims.Role, c.RealIP(), duration, maskedVars)
			} else {
				log.Printf("audit graphql op=%s status=%d actor_id=anonymous actor_role=anonymous ip=%s duration_ms=%d vars=%s", opName, status, c.RealIP(), duration, maskedVars)
			}

			return err
		}
	}
}
