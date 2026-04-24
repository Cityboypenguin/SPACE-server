package main

import (
	"context"
	"fmt"
	"log"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/99designs/gqlgen/graphql/handler"
	"github.com/99designs/gqlgen/graphql/handler/extension"
	"github.com/99designs/gqlgen/graphql/handler/transport"
	"github.com/99designs/gqlgen/graphql/playground"
	"github.com/Cityboypenguin/SPACE-server/graph"
	"github.com/Cityboypenguin/SPACE-server/infra/mysql"
	infraredis "github.com/Cityboypenguin/SPACE-server/infra/redis"
	"github.com/Cityboypenguin/SPACE-server/internal/auth"
	authmiddleware "github.com/Cityboypenguin/SPACE-server/internal/middleware"
	"github.com/Cityboypenguin/SPACE-server/internal/pubsub"
	"github.com/Cityboypenguin/SPACE-server/internal/sse"
	"github.com/Cityboypenguin/SPACE-server/model"
	"github.com/Cityboypenguin/SPACE-server/repository"
	"github.com/Cityboypenguin/SPACE-server/usecase/administrator"
	messageusecase "github.com/Cityboypenguin/SPACE-server/usecase/message"
	profileusecase "github.com/Cityboypenguin/SPACE-server/usecase/profile"
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
	profileRepository := mysql.NewMySQLProfileRepository(database)

	if err := bootstrapInitialAdmin(context.Background(), administratorRepository); err != nil {
		log.Fatalf("failed to bootstrap initial admin: %v", err)
	}

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

	getProfileUseCase := profileusecase.NewGetProfileUseCase(profileRepository)
	updateProfileUseCase := profileusecase.NewUpdateProfileUseCase(profileRepository)

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
	refreshUserTokenUseCase := userusecase.NewRefreshUserTokenUseCase(userRepository, revokedTokenRepository)
	refreshAdministratorTokenUseCase := administrator.NewRefreshAdministratorTokenUseCase(administratorRepository, revokedTokenRepository)
	logoutUserUseCase := userusecase.NewLogoutUserUseCase(revokedTokenRepository)
	logoutAdministratorUseCase := administrator.NewLogoutAdministratorUseCase(revokedTokenRepository)

	sendMessageUseCase := messageusecase.NewSendMessageUseCase(messageRepository)
	listMessagesUseCase := messageusecase.NewListMessagesUseCase(messageRepository)
	createRoomUseCase := roomusecase.NewCreateRoomUseCase(roomRepository)
	getRoomUseCase := roomusecase.NewGetRoomUseCase(roomRepository)
	listMyDMRoomsUseCase := roomusecase.NewListMyDMRoomsUseCase(roomUserRepository)
	getOrCreateDMRoomUseCase := roomusecase.NewGetOrCreateDMRoomUseCase(roomUserRepository)
	addUserToRoomUseCase := roomusecase.NewAddUserToRoomUseCase(roomUserRepository)
	removeUserFromRoomUseCase := roomusecase.NewRemoveUserFromRoomUseCase(roomUserRepository)

	ps := pubsub.New()

	resolver := &graph.Resolver{
		CreateUserUseCase:                createUserUseCase,
		ListUsersUseCase:                 listUsersUseCase,
		DeleteUserUseCase:                deleteUserUsecase,
		UpdateUserUseCase:                updateUserUsecase,
		GetUserByIDUseCase:               getUserByIDUsecase,
		SearchUsersUseCase:               searchUsersUseCase,
		LoginUserUseCase:                 loginUserUseCase,
		RefreshUserTokenUseCase:          refreshUserTokenUseCase,
		LogoutUserUseCase:                logoutUserUseCase,
		GetProfileUseCase:                getProfileUseCase,
		UpdateProfileUseCase:             updateProfileUseCase,
		CreateAdministratorUseCase:       createAdministratorUseCase,
		GetAdministratorByIDUseCase:      getAdministratorByIDUseCase,
		ListAdministratorsUseCase:        listAdministratorsUseCase,
		DeleteAdministratorUseCase:       deleteAdministratorUseCase,
		UpdateAdministratorUseCase:       updateAdministratorUseCase,
		SearchAdministratorsUseCase:      searchAdministratorsUseCase,
		LoginAdministratorUseCase:        loginAdministratorUseCase,
		RefreshAdministratorTokenUseCase: refreshAdministratorTokenUseCase,
		LogoutAdministratorUseCase:       logoutAdministratorUseCase,
		SendMessageUseCase:               sendMessageUseCase,
		ListMessagesUseCase:              listMessagesUseCase,
		CreateRoomUseCase:                createRoomUseCase,
		GetRoomUseCase:                   getRoomUseCase,
		ListMyDMRoomsUseCase:             listMyDMRoomsUseCase,
		GetOrCreateDMRoomUseCase:         getOrCreateDMRoomUseCase,
		AddUserToRoomUseCase:             addUserToRoomUseCase,
		RemoveUserFromRoomUseCase:        removeUserFromRoomUseCase,
		RoomUserRepository:               roomUserRepository,
		UserRepository:                   userRepository,
		PubSub:                           ps,
	}

	// middleware
	e.Use(middleware.RequestLogger())
	e.Use(middleware.Recover())
	// CORSはJWTより先に登録しないと、401レスポンスにCORSヘッダーが付かずブラウザがブロックする
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
	e.Use(authmiddleware.JWTAuth(revokedTokenRepository))
	e.Use(authmiddleware.GraphQLRateLimit())
	e.Use(authmiddleware.GraphQLAudit())

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
				// origin := r.Header.Get("Origin")
				// return origin == "http://localhost:5173"
				return true // 開発環境では全てのオリジンを許可
			},
			ReadBufferSize:  1024,
			WriteBufferSize: 1024,
		},
		KeepAlivePingInterval: 10 * time.Second,
		InitFunc: func(ctx context.Context, initPayload transport.InitPayload) (context.Context, *transport.InitPayload, error) {
			if _, ok := auth.ClaimsFromContext(ctx); ok {
				return ctx, nil, nil
			}

			authHeader := authHeaderFromInitPayload(initPayload)
			if strings.TrimSpace(authHeader) == "" {
				return nil, nil, fmt.Errorf("missing authorization in websocket init payload")
			}

			tokenStr := strings.TrimSpace(strings.TrimPrefix(authHeader, "Bearer "))
			if tokenStr == "" {
				return nil, nil, fmt.Errorf("missing bearer token in websocket init payload")
			}

			claims, err := auth.ValidateAccessToken(tokenStr)
			if err != nil {
				return nil, nil, fmt.Errorf("invalid token")
			}

			revoked, err := revokedTokenRepository.IsRevoked(ctx, tokenStr)
			if err != nil {
				return nil, nil, fmt.Errorf("failed to verify token")
			}
			if revoked {
				return nil, nil, fmt.Errorf("token has been revoked")
			}

			ctx = auth.WithClaims(ctx, claims)
			return ctx, nil, nil
		},
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

func authHeaderFromInitPayload(initPayload transport.InitPayload) string {
	for _, key := range []string{"Authorization", "authorization", "authToken", "token", "accessToken"} {
		raw, ok := initPayload[key]
		if !ok {
			continue
		}

		header, ok := raw.(string)
		if !ok {
			continue
		}

		header = strings.TrimSpace(header)
		if header != "" {
			return header
		}
	}

	for _, containerKey := range []string{"headers", "header"} {
		rawContainer, ok := initPayload[containerKey]
		if !ok {
			continue
		}

		switch container := rawContainer.(type) {
		case map[string]interface{}:
			for _, key := range []string{"Authorization", "authorization", "authToken", "token", "accessToken"} {
				raw, ok := container[key]
				if !ok {
					continue
				}
				header, ok := raw.(string)
				if !ok {
					continue
				}
				header = strings.TrimSpace(header)
				if header != "" {
					return header
				}
			}
		case map[string]string:
			for _, key := range []string{"Authorization", "authorization", "authToken", "token", "accessToken"} {
				header := strings.TrimSpace(container[key])
				if header != "" {
					return header
				}
			}
		}
	}

	return ""
}

// bootstrapInitialAdmin creates exactly one initial admin when env vars are provided.
// If INIT_ADMIN_EMAIL/INIT_ADMIN_PASSWORD are empty, this step is skipped.
func bootstrapInitialAdmin(ctx context.Context, adminRepo repository.AdministratorRepository) error {
	name := strings.TrimSpace(os.Getenv("INIT_ADMIN_NAME"))
	email := strings.TrimSpace(os.Getenv("INIT_ADMIN_EMAIL"))
	password := os.Getenv("INIT_ADMIN_PASSWORD")

	if email == "" || password == "" {
		return nil
	}
	if name == "" {
		name = "Initial Admin"
	}

	existing, err := adminRepo.FindByEmail(ctx, email)
	if err != nil {
		return err
	}
	if existing != nil {
		return nil
	}

	admin := &model.Administrator{}
	if err := admin.CreateAdministrator(model.CreateAdministratorParam{
		Name:     name,
		Email:    email,
		Password: password,
	}); err != nil {
		return err
	}

	if err := adminRepo.SaveAdministrator(ctx, admin); err != nil {
		return err
	}

	log.Printf("initial administrator created: %s", email)
	return nil
}
