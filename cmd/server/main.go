package main

import (
	"os"

	"github.com/99designs/gqlgen/graphql/handler"
	"github.com/99designs/gqlgen/graphql/playground"
	"github.com/Cityboypenguin/SPACE-server/graph"
	"github.com/Cityboypenguin/SPACE-server/infra/inmem"
	"github.com/Cityboypenguin/SPACE-server/internal/sse"
	"github.com/Cityboypenguin/SPACE-server/usecase/user"
	"github.com/labstack/echo/v4"
	"github.com/labstack/echo/v4/middleware"
)

func main() {
	port := os.Getenv("PORT")
	if port == "" {
		port = "8080"
	}

	e := echo.New()

	// middleware
	e.Use(middleware.RequestLogger())
	e.Use(middleware.Recover())

	// テスト用エンドポイント
	e.GET("/", func(c echo.Context) error {
		return c.String(200, "test message: Hello from SPACE Server!")
	})

	userRepo := inmem.NewInmemUserRepository()
	resolver := &graph.Resolver{
		SignUpUseCase: &user.SignUpInteractor{
			UserRepository: userRepo,
		},
		GetUserUseCase: &user.GetUserInteractor{
			UserRepository: userRepo,
		},
		GetUsersUseCase: &user.GetUsersInteractor{
			UserRepository: userRepo,
		},
	}

	// GraphQL server
	gqlServer := handler.NewDefaultServer(
		graph.NewExecutableSchema(
			graph.Config{
				Resolvers: resolver,
			},
		),
	)

	// GraphQL エンドポイント
	e.POST("/query", func(c echo.Context) error {
		gqlServer.ServeHTTP(c.Response(), c.Request())
		return nil
	})
	e.GET("/query", func(c echo.Context) error {
		gqlServer.ServeHTTP(c.Response(), c.Request())
		return nil
	})

	// Playground（開発用）
	e.GET("/playground", func(c echo.Context) error {
		playground.Handler("GraphQL Playground", "/query").
			ServeHTTP(c.Response(), c.Request())
		return nil
	})

	hub := sse.NewHub()

	// SSE
	e.GET("/events", sse.NewHandler(hub))

	e.Logger.Fatal(e.Start(":" + port))
}
