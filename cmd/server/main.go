package main

import (
	"log"
	"net/http"
	"time"

	"github.com/99designs/gqlgen/graphql/handler"
	"github.com/99designs/gqlgen/graphql/handler/extension"
	"github.com/99designs/gqlgen/graphql/handler/transport"
	"github.com/99designs/gqlgen/graphql/playground"
	"github.com/Cityboypenguin/SPACE-server/graph"
	"github.com/Cityboypenguin/SPACE-server/infra/mysql"
	infraredis "github.com/Cityboypenguin/SPACE-server/infra/redis"
	"github.com/Cityboypenguin/SPACE-server/internal/pubsub"
	"github.com/Cityboypenguin/SPACE-server/internal/sse"
	"github.com/Cityboypenguin/SPACE-server/usecase/administrator"
	messageusecase "github.com/Cityboypenguin/SPACE-server/usecase/message"
	roomusecase "github.com/Cityboypenguin/SPACE-server/usecase/room"
	userusecase "github.com/Cityboypenguin/SPACE-server/usecase/user"
	"github.com/gorilla/websocket"
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
	messageRepository := mysql.NewMySQLMessageRepository(database)
	roomRepository := mysql.NewMySQLRoomRepository(database)
	roomUserRepository := mysql.NewMySQLRoomUserRepository(database)

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

	redisClient, err := infraredis.New()
	if err != nil {
		log.Fatalf("failed to connect to redis: %v", err)
	}
	defer redisClient.Close()
	revokedTokenRepository := infraredis.NewRedisRevokedTokenRepository(redisClient)
	logoutUserUseCase := userusecase.NewLogoutUserUseCase(revokedTokenRepository)
	logoutAdministratorUseCase := administrator.NewLogoutAdministratorUseCase(revokedTokenRepository)

	sendMessageUseCase := messageusecase.NewSendMessageUseCase(messageRepository)
	listMessagesUseCase := messageusecase.NewListMessagesUseCase(messageRepository)
	createRoomUseCase := roomusecase.NewCreateRoomUseCase(roomRepository)
	getRoomUseCase := roomusecase.NewGetRoomUseCase(roomRepository)
	getOrCreateDMRoomUseCase := roomusecase.NewGetOrCreateDMRoomUseCase(roomRepository, roomUserRepository)
	addUserToRoomUseCase := roomusecase.NewAddUserToRoomUseCase(roomUserRepository)
	removeUserFromRoomUseCase := roomusecase.NewRemoveUserFromRoomUseCase(roomUserRepository)

	ps := pubsub.New()

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
		SendMessageUseCase:          sendMessageUseCase,
		ListMessagesUseCase:         listMessagesUseCase,
		CreateRoomUseCase:           createRoomUseCase,
		GetRoomUseCase:              getRoomUseCase,
		GetOrCreateDMRoomUseCase:    getOrCreateDMRoomUseCase,
		AddUserToRoomUseCase:        addUserToRoomUseCase,
		RemoveUserFromRoomUseCase:   removeUserFromRoomUseCase,
		PubSub:                      ps,
	}

	// middleware
	e.Use(middleware.RequestLogger())
	e.Use(middleware.Recover())

	// CORS設定:フロントエンド(localhost:5173)からの通信を許可
	e.Use(middleware.CORSWithConfig(middleware.CORSConfig{
		AllowOrigins: []string{"http://localhost:5173"},
		AllowHeaders: []string{
			echo.HeaderOrigin,
			echo.HeaderContentType,
			echo.HeaderAccept,
			echo.HeaderAuthorization,
			"Sec-WebSocket-Protocol",
		},
		AllowMethods: []string{"GET", "POST", "OPTIONS"},
	}))

	// テスト用エンドポイント
	e.GET("/", func(c echo.Context) error {
		return c.String(200, "test message: Hello from SPACE Server!")
	})

	// GraphQL server with WebSocket transport
	gqlServer := handler.New(
		graph.NewExecutableSchema(
			graph.Config{
				Resolvers: resolver,
			},
		),
	)
	gqlServer.AddTransport(transport.Websocket{
		Upgrader: websocket.Upgrader{
			CheckOrigin: func(r *http.Request) bool {
				origin := r.Header.Get("Origin")
				return origin == "http://localhost:5173" || origin == ""
			},
			ReadBufferSize:  1024,
			WriteBufferSize: 1024,
		},
		KeepAlivePingInterval: 10 * time.Second,
	})
	gqlServer.AddTransport(transport.Options{})
	gqlServer.AddTransport(transport.GET{})
	gqlServer.AddTransport(transport.POST{})
	gqlServer.AddTransport(transport.MultipartForm{})
	gqlServer.Use(extension.Introspection{})

	// GraphQL エンドポイント (POST: query/mutation, GET: WebSocket subscription)
	gqlHandler := func(c echo.Context) error {
		gqlServer.ServeHTTP(c.Response(), c.Request())
		return nil
	}
	e.POST("/query", gqlHandler)
	e.GET("/query", gqlHandler)

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
