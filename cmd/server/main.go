package main

import (
	"log"

	"github.com/99designs/gqlgen/graphql/handler"
	"github.com/99designs/gqlgen/graphql/playground"
	"github.com/Cityboypenguin/SPACE-server/graph"
	"github.com/Cityboypenguin/SPACE-server/infra/mysql"
	infraredis "github.com/Cityboypenguin/SPACE-server/infra/redis"
	"github.com/Cityboypenguin/SPACE-server/internal/sse"
	"github.com/Cityboypenguin/SPACE-server/usecase/administrator"
	commentusecase "github.com/Cityboypenguin/SPACE-server/usecase/comment"
	favoriteusecase "github.com/Cityboypenguin/SPACE-server/usecase/favorite"
	postusecase "github.com/Cityboypenguin/SPACE-server/usecase/post"
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
	administratorRepository := mysql.NewMySQLAdministratorRepository(database)
	postRepository := mysql.NewMySQLPostRepository(database)
	commentRepository := mysql.NewMySQLCommentRepository(database)
	favoriteRepository := mysql.NewMySQLFavoriteRepository(database)
	createUserUseCase := userusecase.NewCreateUserUseCase(userRepository)
	listUsersUseCase := userusecase.NewListUsersUseCase(userRepository)
	deleteUserUsecase := userusecase.NewDeleteUserUseCase(userRepository)
	updateUserUsecase := userusecase.NewUpdateUserUseCase(userRepository)
	getUserByIDUsecase := userusecase.NewGetUserByIDUseCase(userRepository)
	searchUsersUseCase := userusecase.NewSearchUsersUseCase(userRepository)
	loginUserUseCase := userusecase.NewLoginUserUseCase(userRepository)
	createAdministratorUseCase := administrator.NewCreateAdministratorUseCase(administratorRepository)
	getAdministratorByIDUseCase := administrator.NewGetAdministratorByIDUseCase(administratorRepository)
	listAdministratorsUseCase := administrator.NewListAdministratorsUseCase(administratorRepository)
	deleteAdministratorUseCase := administrator.NewDeleteAdministratorUseCase(administratorRepository)
	updateAdministratorUseCase := administrator.NewUpdateAdministratorUseCase(administratorRepository)
	searchAdministratorsUseCase := administrator.NewSearchAdministratorsUseCase(administratorRepository)
	loginAdministratorUseCase := administrator.NewLoginAdministratorUseCase(administratorRepository)
	createPostUsecase := postusecase.NewCreatePostUseCase(postRepository)
	updatePostUsecase := postusecase.NewUpdatePostUseCase(postRepository)
	deletePostUsecase := postusecase.NewDeletePostUseCase(postRepository)
	getPostByIDUsecase := postusecase.NewGetPostByIDUseCase(postRepository)
	listPostsUsecase := postusecase.NewListPostsUseCase(postRepository)
	searchPostsUsecase := postusecase.NewSearchPostsUseCase(postRepository)
	getPostsByUserIDUsecase := postusecase.NewGetPostsByUserIDUseCase(postRepository)

	createCommentUsecase := commentusecase.NewCreateCommentUseCase(commentRepository)
	updateCommentUsecase := commentusecase.NewUpdateCommentUseCase(commentRepository)
	deleteCommentUsecase := commentusecase.NewDeleteCommentUseCase(commentRepository)
	getCommentByIDUsecase := commentusecase.NewGetCommentByIDUseCase(commentRepository)
	getCommentByPostIDUsecase := commentusecase.NewGetCommentsByPostIDUseCase(commentRepository)

	createFavoriteUsecase := favoriteusecase.NewCreateFavoriteUseCase(favoriteRepository)
	deleteFavoriteUsecase := favoriteusecase.NewDeleteFavoriteUseCase(favoriteRepository)
	getFavoriteByIDUsecase := favoriteusecase.NewGetFavoriteByIDUseCase(favoriteRepository)
	getFavoritesByPostIDUsecase := favoriteusecase.NewGetFavoritesByPostIDUseCase(favoriteRepository)

	redisClient, err := infraredis.New()
	if err != nil {
		log.Fatalf("failed to connect to redis: %v", err)
	}
	defer redisClient.Close()
	revokedTokenRepository := infraredis.NewRedisRevokedTokenRepository(redisClient)
	logoutUserUseCase := userusecase.NewLogoutUserUseCase(revokedTokenRepository)
	logoutAdministratorUseCase := administrator.NewLogoutAdministratorUseCase(revokedTokenRepository)

	resolver := &graph.Resolver{
		CreateUserUseCase:           createUserUseCase,
		ListUsersUseCase:            listUsersUseCase,
		DeleteUserUseCase:           deleteUserUsecase,
		UpdateUserUseCase:           updateUserUsecase,
		GetUserByIDUseCase:          getUserByIDUsecase,
		SearchUsersUseCase:          searchUsersUseCase,
		LoginUserUseCase:            loginUserUseCase,
		LogoutUserUseCase:           logoutUserUseCase,
		CreateAdministratorUseCase:  createAdministratorUseCase,
		GetAdministratorByIDUseCase: getAdministratorByIDUseCase,
		ListAdministratorsUseCase:   listAdministratorsUseCase,
		DeleteAdministratorUseCase:  deleteAdministratorUseCase,
		UpdateAdministratorUseCase:  updateAdministratorUseCase,
		SearchAdministratorsUseCase: searchAdministratorsUseCase,
		LoginAdministratorUseCase:   loginAdministratorUseCase,
		LogoutAdministratorUseCase:  logoutAdministratorUseCase,
		CreatePostUseCase:           createPostUsecase,
		UpdatePostUseCase:           updatePostUsecase,
		DeletePostUseCase:           deletePostUsecase,
		GetPostByIDUseCase:          getPostByIDUsecase,
		ListPostsUseCase:            listPostsUsecase,
		SearchPostsUseCase:          searchPostsUsecase,
		GetPostsByUserIDUseCase:     getPostsByUserIDUsecase,
		CreateCommentUseCase:        createCommentUsecase,
		UpdateCommentUseCase:        updateCommentUsecase,
		DeleteCommentUseCase:        deleteCommentUsecase,
		GetCommentByIDUseCase:       getCommentByIDUsecase,
		GetCommentsByPostIDUseCase:  getCommentByPostIDUsecase,
		CreateFavoriteUseCase:       createFavoriteUsecase,
		DeleteFavoriteUseCase:       deleteFavoriteUsecase,
		GetFavoriteByIDUseCase:      getFavoriteByIDUsecase,
		GetFavoritesByPostIDUseCase: getFavoritesByPostIDUsecase,
	}

	// middleware
	e.Use(middleware.RequestLogger())
	e.Use(middleware.Recover())

	// CORS設定:フロントエンド(localhost:5173)からの通信を許可
	e.Use(middleware.CORSWithConfig(middleware.CORSConfig{
		AllowOrigins: []string{"http://localhost:5173"},
		AllowHeaders: []string{echo.HeaderOrigin, echo.HeaderContentType, echo.HeaderAccept, echo.HeaderAuthorization},
	}))

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
