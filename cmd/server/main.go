package main

import (
	"log"

	"github.com/99designs/gqlgen/graphql/handler"
	"github.com/99designs/gqlgen/graphql/playground"
	"github.com/Cityboypenguin/SPACE-server/graph"
	"github.com/Cityboypenguin/SPACE-server/infra/mysql"
	"github.com/Cityboypenguin/SPACE-server/internal/sse"
	userusecase "github.com/Cityboypenguin/SPACE-server/usecase/user"
	"github.com/labstack/echo/v4"
	"github.com/labstack/echo/v4/middleware"
)

func main() {
	database, err := mysql.New()
	if err != nil {
		log.Fatalf("failed to connect to database: %v", err)
	}
	defer database.Close()

	e := echo.New()

	userRepository := mysql.NewMySQLUserRepository(database)
	createUserUseCase := userusecase.NewCreateUserUseCase(userRepository)
	listUsersUseCase := userusecase.NewListUsersUseCase(userRepository)
	deleteUserUsecase := userusecase.NewDeleteUserUseCase(userRepository)

	resolver := &graph.Resolver{
		CreateUserUseCase: createUserUseCase,
		ListUsersUseCase:  listUsersUseCase,
		DeleteUserUseCase: deleteUserUsecase,
	}

	// middleware
	e.Use(middleware.RequestLogger())
	e.Use(middleware.Recover())

	// テスト用エンドポイント
	e.GET("/", func(c echo.Context) error {
		return c.String(200, "test message: Hello from SPACE Server!")
	})

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

	// Playground（開発用）
	e.GET("/playground", func(c echo.Context) error {
		playground.Handler("GraphQL Playground", "/query").
			ServeHTTP(c.Response(), c.Request())
		return nil
	})

	hub := sse.NewHub()
	// SSE
	e.GET("/events", sse.NewHandler(hub))
	e.Logger.Fatal(e.Start(":8080"))
}
