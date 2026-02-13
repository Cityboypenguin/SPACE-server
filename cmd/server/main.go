package main

import (
	"github.com/99designs/gqlgen/graphql/handler"
	"github.com/99designs/gqlgen/graphql/playground"
	"github.com/Cityboypenguin/SPACE-server/graph"
	"github.com/Cityboypenguin/SPACE-server/internal/sse"
	"github.com/labstack/echo/v4"
	"github.com/labstack/echo/v4/middleware"
)

func main() {
	e := echo.New()

	// ミドルウェア（ログとか）
	e.Use(middleware.RequestLogger())
	e.Use(middleware.Recover())

	// GraphQLサーバーの設定
	// ※ここで Resolver{} を渡すだけでOKになりました
	gqlServer := handler.NewDefaultServer(
		graph.NewExecutableSchema(
			graph.Config{
				Resolvers: &graph.Resolver{},
			},
		),
	)

	// エンドポイントの設定
	e.POST("/query", func(c echo.Context) error {
		gqlServer.ServeHTTP(c.Response(), c.Request())
		return nil
	})

	e.GET("/playground", func(c echo.Context) error {
		playground.Handler("GraphQL Playground", "/query").ServeHTTP(c.Response(), c.Request())
		return nil
	})

	// SSE（通知機能）の設定
	hub := sse.NewHub()
	e.GET("/events", sse.NewHandler(hub))

	// サーバー起動
	e.Logger.Fatal(e.Start(":8080"))
}
