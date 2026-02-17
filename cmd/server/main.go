package main

import (
	"github.com/99designs/gqlgen/graphql/handler"
	"github.com/99designs/gqlgen/graphql/playground"
	"github.com/Cityboypenguin/SPACE-server/graph"
	"github.com/Cityboypenguin/SPACE-server/repository" // 追加
	"github.com/Cityboypenguin/SPACE-server/usecase"    // 追加
	"github.com/labstack/echo/v4"
	"github.com/labstack/echo/v4/middleware"
)

func main() {
	e := echo.New()

	e.Use(middleware.CORSWithConfig(middleware.CORSConfig{
		AllowOrigins: []string{"*"},
		AllowMethods: []string{echo.GET, echo.POST, echo.PUT, echo.DELETE},
		AllowHeaders: []string{echo.HeaderOrigin, echo.HeaderContentType, echo.HeaderAccept},
	}))
	e.Use(middleware.RequestLogger())
	e.Use(middleware.Recover())

	// ★ 1. 冷蔵庫（Repo）を作る
	userRepo := repository.NewUserRepo()
	postRepo := repository.NewPostRepo()

	// ★ 2. シェフ（Usecase）を作る（冷蔵庫を渡す）
	userUsecase := usecase.NewUserUsecase(userRepo)
	postUsecase := usecase.NewPostUsecase(postRepo)

	// ★ 3. サーバー（Resolver）にセットする（シェフを渡す）
	gqlServer := handler.NewDefaultServer(
		graph.NewExecutableSchema(
			graph.Config{
				Resolvers: &graph.Resolver{
					UserUsecase: userUsecase,
					PostUsecase: postUsecase,
				},
			},
		),
	)

	e.POST("/query", func(c echo.Context) error {
		gqlServer.ServeHTTP(c.Response(), c.Request())
		return nil
	})

	e.GET("/playground", func(c echo.Context) error {
		playground.Handler("GraphQL Playground", "/query").ServeHTTP(c.Response(), c.Request())
		return nil
	})

	e.Logger.Fatal(e.Start(":8080"))
}
