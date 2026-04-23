package middleware

import (
	"bytes"
	"encoding/json"
	"io"
	"log"
	"time"

	"github.com/Cityboypenguin/SPACE-server/internal/auth"
	"github.com/labstack/echo/v4"
)

type graphqlRequestPayload struct {
	OperationName string `json:"operationName"`
	Query         string `json:"query"`
}

func GraphQLAudit() echo.MiddlewareFunc {
	return func(next echo.HandlerFunc) echo.HandlerFunc {
		return func(c echo.Context) error {
			if c.Path() != "/query" {
				return next(c)
			}

			start := time.Now()
			opName := c.QueryParam("operationName")

			if c.Request().Method == echo.POST {
				bodyBytes, _ := io.ReadAll(c.Request().Body)
				c.Request().Body = io.NopCloser(bytes.NewBuffer(bodyBytes))

				if len(bodyBytes) > 0 {
					var payload graphqlRequestPayload
					if err := json.Unmarshal(bodyBytes, &payload); err == nil && payload.OperationName != "" {
						opName = payload.OperationName
					}
				}
			}

			err := next(c)
			status := c.Response().Status
			duration := time.Since(start).Milliseconds()

			claims, ok := auth.ClaimsFromContext(c.Request().Context())
			if ok {
				log.Printf("audit graphql op=%s status=%d actor_id=%d actor_role=%s ip=%s duration_ms=%d", opName, status, claims.ID, claims.Role, c.RealIP(), duration)
			} else {
				log.Printf("audit graphql op=%s status=%d actor_id=anonymous actor_role=anonymous ip=%s duration_ms=%d", opName, status, c.RealIP(), duration)
			}

			return err
		}
	}
}
