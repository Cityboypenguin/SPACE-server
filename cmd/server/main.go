package main

import (
	"context"
	"fmt"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/99designs/gqlgen/graphql/handler"
	"github.com/99designs/gqlgen/graphql/handler/extension"
	"github.com/99designs/gqlgen/graphql/handler/transport"
	"github.com/99designs/gqlgen/graphql/playground"
	"github.com/Cityboypenguin/SPACE-server/graph"
	azurerepo "github.com/Cityboypenguin/SPACE-server/infra/azure"
	miniorepo "github.com/Cityboypenguin/SPACE-server/infra/minio"
	"github.com/Cityboypenguin/SPACE-server/infra/mysql"
	infraredis "github.com/Cityboypenguin/SPACE-server/infra/redis"
	"github.com/Cityboypenguin/SPACE-server/internal/auth"
	"github.com/Cityboypenguin/SPACE-server/internal/logger"
	authmiddleware "github.com/Cityboypenguin/SPACE-server/internal/middleware"
	"github.com/Cityboypenguin/SPACE-server/internal/pubsub"
	"github.com/Cityboypenguin/SPACE-server/internal/sse"
	"github.com/Cityboypenguin/SPACE-server/model"
	"github.com/Cityboypenguin/SPACE-server/repository"
	"github.com/Cityboypenguin/SPACE-server/usecase/administrator"
	communityusecase "github.com/Cityboypenguin/SPACE-server/usecase/community"
	favoriteusecase "github.com/Cityboypenguin/SPACE-server/usecase/favorite"
	messageusecase "github.com/Cityboypenguin/SPACE-server/usecase/message"
	postusecase "github.com/Cityboypenguin/SPACE-server/usecase/post"
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
		logger.Log.Fatal().Err(err).Msg("failed to connect to database")
	}

	e := echo.New()

	userRepository := mysql.NewMySQLUserRepository(database)
	administratorRepository := mysql.NewMySQLAdministratorRepository(database)
	postRepository := mysql.NewMySQLPostRepository(database)
	favoriteRepository := mysql.NewMySQLFavoriteRepository(database)
	profileRepository := mysql.NewMySQLProfileRepository(database)
	txManager := mysql.NewMySQLTxManager(database)

	if err := bootstrapInitialAdmin(context.Background(), administratorRepository); err != nil {
		logger.Log.Fatal().Err(err).Msg("failed to bootstrap initial admin")
	}

	messageRepository := mysql.NewMySQLMessageRepository(database)
	roomRepository := mysql.NewMySQLRoomRepository(database)
	roomUserRepository := mysql.NewMySQLRoomUserRepository(database)

	var storageRepository repository.StorageRepository
	if os.Getenv("STORAGE_PROVIDER") == "azure" {
		storageRepository, err = azurerepo.New()
		if err != nil {
			logger.Log.Fatal().Err(err).Msg("failed to connect to azure blob storage")
		}
	} else {
		storageRepository, err = miniorepo.New()
		if err != nil {
			logger.Log.Fatal().Err(err).Msg("failed to connect to minio")
		}
	}

	createUserUseCase := userusecase.NewCreateUserUseCase(userRepository, profileRepository, txManager)
	listUsersUseCase := userusecase.NewListUsersUseCase(userRepository)
	deleteUserUseCase := userusecase.NewDeleteUserUseCase(userRepository, postRepository)
	updateUserUseCase := userusecase.NewUpdateUserUseCase(userRepository)
	getUserByIDUseCase := userusecase.NewGetUserByIDUseCase(userRepository)
	searchUsersUseCase := userusecase.NewSearchUsersUseCase(userRepository)
	loginUserUseCase := userusecase.NewLoginUserUseCase(userRepository)
	freezeUserUseCase := userusecase.NewFreezeUserUseCase(userRepository)
	unfreezeUserUseCase := userusecase.NewUnfreezeUserUseCase(userRepository)
	getProfileUseCase := profileusecase.NewGetProfileUseCase(profileRepository)
	updateProfileUseCase := profileusecase.NewUpdateProfileUseCase(profileRepository)
	setAvatarUseCase := profileusecase.NewSetAvatarUseCase(profileRepository)

	createAdministratorUseCase := administrator.NewCreateAdministratorUseCase(administratorRepository)
	countAdministratorsUseCase := administrator.NewCountAdministratorsUseCase(administratorRepository)
	getAdministratorByIDUseCase := administrator.NewGetAdministratorByIDUseCase(administratorRepository)
	listAdministratorsUseCase := administrator.NewListAdministratorsUseCase(administratorRepository)
	deleteAdministratorUseCase := administrator.NewDeleteAdministratorUseCase(administratorRepository)
	updateAdministratorUseCase := administrator.NewUpdateAdministratorUseCase(administratorRepository)
	searchAdministratorsUseCase := administrator.NewSearchAdministratorsUseCase(administratorRepository)
	loginAdministratorUseCase := administrator.NewLoginAdministratorUseCase(administratorRepository)

	createPostUseCase := postusecase.NewCreatePostUseCase(postRepository)
	updatePostUseCase := postusecase.NewUpdatePostUseCase(postRepository)
	deletePostUseCase := postusecase.NewDeletePostUseCase(postRepository)
	getPostByIDUseCase := postusecase.NewGetPostByIDUseCase(postRepository)
	listPostsUseCase := postusecase.NewListPostsUseCase(postRepository)
	searchPostsUseCase := postusecase.NewSearchPostsUseCase(postRepository)
	getPostsByUserIDUseCase := postusecase.NewGetPostsByUserIDUseCase(postRepository)
	getRepliesByIDUseCase := postusecase.NewGetRepliesByIDUseCase(postRepository)
	listTopLevelPostsUseCase := postusecase.NewListTopLevelPostsUseCase(postRepository)

	createFavoriteUseCase := favoriteusecase.NewCreateFavoriteUseCase(favoriteRepository, postRepository)
	deleteFavoriteUseCase := favoriteusecase.NewDeleteFavoriteUseCase(favoriteRepository, postRepository)
	deleteFavoriteByUserIDAndPostIDUseCase := favoriteusecase.NewDeleteFavoriteByUserIDAndPostIDUseCase(favoriteRepository, postRepository)
	getFavoriteByIDUseCase := favoriteusecase.NewGetFavoriteByIDUseCase(favoriteRepository)
	getFavoritesByPostIDUseCase := favoriteusecase.NewGetFavoritesByPostIDUseCase(favoriteRepository)
	getFavoritesByUserIDUseCase := favoriteusecase.NewGetFavoritesByUserIDUseCase(favoriteRepository)
	getFavoriteByUserIDAndPostIDUseCase := favoriteusecase.NewGetFavoriteByUserIDAndPostIDUseCase(favoriteRepository)
	listFavoritesUseCase := favoriteusecase.NewListFavoritesUseCase(favoriteRepository)

	redisClient, err := infraredis.New()
	if err != nil {
		logger.Log.Fatal().Err(err).Msg("failed to connect to redis")
	}
	revokedTokenRepository := infraredis.NewRedisRevokedTokenRepository(redisClient)
	refreshUserTokenUseCase := userusecase.NewRefreshUserTokenUseCase(userRepository, revokedTokenRepository)
	refreshAdministratorTokenUseCase := administrator.NewRefreshAdministratorTokenUseCase(administratorRepository, revokedTokenRepository)
	logoutUserUseCase := userusecase.NewLogoutUserUseCase(revokedTokenRepository)
	logoutAdministratorUseCase := administrator.NewLogoutAdministratorUseCase(revokedTokenRepository)

	getMessageByIDUseCase := messageusecase.NewGetMessageByIDUseCase(messageRepository)
	sendMessageUseCase := messageusecase.NewSendMessageUseCase(messageRepository)
	listMessagesUseCase := messageusecase.NewListMessagesUseCase(messageRepository)
	deleteMessageUseCase := messageusecase.NewDeleteMessageUseCase(messageRepository)
	updateMessageUseCase := messageusecase.NewUpdateMessageUseCase(messageRepository)
	createRoomUseCase := roomusecase.NewCreateRoomUseCase(roomRepository)
	getRoomUseCase := roomusecase.NewGetRoomUseCase(roomRepository)
	deleteRoomUseCase := roomusecase.NewDeleteRoomUseCase(roomRepository)
	getUserIDsByRoomIDUseCase := roomusecase.NewGetUserIDsByRoomIDUseCase(roomUserRepository)
	listUsersByRoomIDsUseCase := roomusecase.NewListUsersByRoomIDsUseCase(roomUserRepository)
	listMyDMRoomsUseCase := roomusecase.NewListMyDMRoomsUseCase(roomUserRepository)
	getOrCreateDMRoomUseCase := roomusecase.NewGetOrCreateDMRoomUseCase(roomUserRepository)
	addUserToRoomUseCase := roomusecase.NewAddUserToRoomUseCase(roomUserRepository)
	removeUserFromRoomUseCase := roomusecase.NewRemoveUserFromRoomUseCase(roomUserRepository)
	joinRoomUseCase := roomusecase.NewJoinRoomUseCase(roomRepository, roomUserRepository)
	getRoomUserRoleUseCase := roomusecase.NewGetRoomUserRoleUseCase(roomUserRepository)
	setRoomUserRoleUseCase := roomusecase.NewSetRoomUserRoleUseCase(roomUserRepository)
	listRoomMembersWithRolesUseCase := roomusecase.NewListRoomMembersWithRolesUseCase(roomUserRepository)

	communityRepository := mysql.NewMySQLCommunityRepository(database)
	createCommunityUseCase := communityusecase.NewCreateCommunityUseCase(communityRepository)
	getCommunityUseCase := communityusecase.NewGetCommunityUseCase(communityRepository)
	updateCommunityUseCase := communityusecase.NewUpdateCommunityUseCase(communityRepository)
	searchCommunityUseCase := communityusecase.NewSearchCommunityUseCase(communityRepository)
	listMyCommunitiesUseCase := communityusecase.NewListMyCommunitiesUseCase(communityRepository)
	listAllCommunitiesUseCase := communityusecase.NewListAllCommunitiesUseCase(communityRepository)
	promoteToCommunityOwnerUseCase := communityusecase.NewPromoteToCommunityOwnerUseCase(communityRepository, roomUserRepository)
	demoteFromCommunityOwnerUseCase := communityusecase.NewDemoteFromCommunityOwnerUseCase(communityRepository, roomUserRepository)
	isSoleOwnerWithOtherMembersUseCase := communityusecase.NewIsSoleOwnerWithOtherMembersUseCase(communityRepository)

	ps := pubsub.New()

	resolver := &graph.Resolver{
		StorageRepository: storageRepository,

		CreateUserUseCase:       createUserUseCase,
		ListUsersUseCase:        listUsersUseCase,
		DeleteUserUseCase:       deleteUserUseCase,
		UpdateUserUseCase:       updateUserUseCase,
		GetUserByIDUseCase:      getUserByIDUseCase,
		SearchUsersUseCase:      searchUsersUseCase,
		LoginUserUseCase:        loginUserUseCase,
		RefreshUserTokenUseCase: refreshUserTokenUseCase,
		LogoutUserUseCase:       logoutUserUseCase,
		FreezeUserUseCase:       freezeUserUseCase,
		UnfreezeUserUseCase:     unfreezeUserUseCase,
		SetAvatarUseCase:        setAvatarUseCase,

		GetProfileUseCase:    getProfileUseCase,
		UpdateProfileUseCase: updateProfileUseCase,

		GetAdministratorByIDUseCase:      getAdministratorByIDUseCase,
		CreateAdministratorUseCase:       createAdministratorUseCase,
		CountAdministratorsUseCase:       countAdministratorsUseCase,
		ListAdministratorsUseCase:        listAdministratorsUseCase,
		DeleteAdministratorUseCase:       deleteAdministratorUseCase,
		UpdateAdministratorUseCase:       updateAdministratorUseCase,
		SearchAdministratorsUseCase:      searchAdministratorsUseCase,
		LoginAdministratorUseCase:        loginAdministratorUseCase,
		RefreshAdministratorTokenUseCase: refreshAdministratorTokenUseCase,
		LogoutAdministratorUseCase:       logoutAdministratorUseCase,

		GetPostByIDUseCase:       getPostByIDUseCase,
		CreatePostUseCase:        createPostUseCase,
		ListPostsUseCase:         listPostsUseCase,
		DeletePostUseCase:        deletePostUseCase,
		UpdatePostUseCase:        updatePostUseCase,
		SearchPostsUseCase:       searchPostsUseCase,
		ListTopLevelPostsUseCase: listTopLevelPostsUseCase,
		GetRepliesByIDUseCase:    getRepliesByIDUseCase,
		GetPostsByUserIDUseCase:  getPostsByUserIDUseCase,

		GetFavoriteByIDUseCase:                 getFavoriteByIDUseCase,
		CreateFavoriteUseCase:                  createFavoriteUseCase,
		DeleteFavoriteUseCase:                  deleteFavoriteUseCase,
		DeleteFavoriteByUserIDAndPostIDUseCase: deleteFavoriteByUserIDAndPostIDUseCase,
		GetFavoriteByUserIDAndPostIDUseCase:    getFavoriteByUserIDAndPostIDUseCase,
		GetFavoritesByPostIDUseCase:            getFavoritesByPostIDUseCase,
		GetFavoritesByUserIDUseCase:            getFavoritesByUserIDUseCase,
		ListFavoritesUseCase:                   listFavoritesUseCase,

		GetMessageByIDUseCase:     getMessageByIDUseCase,
		SendMessageUseCase:        sendMessageUseCase,
		ListMessagesUseCase:       listMessagesUseCase,
		DeleteMessageUseCase:      deleteMessageUseCase,
		UpdateMessageUseCase:      updateMessageUseCase,
		CreateRoomUseCase:         createRoomUseCase,
		GetRoomUseCase:            getRoomUseCase,
		DeleteRoomUseCase:         deleteRoomUseCase,
		GetUserIDsByRoomIDUseCase: getUserIDsByRoomIDUseCase,
		ListUsersByRoomIDsUseCase: listUsersByRoomIDsUseCase,
		ListMyDMRoomsUseCase:      listMyDMRoomsUseCase,
		GetOrCreateDMRoomUseCase:  getOrCreateDMRoomUseCase,
		AddUserToRoomUseCase:      addUserToRoomUseCase,
		RemoveUserFromRoomUseCase: removeUserFromRoomUseCase,
		JoinRoomUseCase:           joinRoomUseCase,
		GetRoomUserRoleUseCase:          getRoomUserRoleUseCase,
		SetRoomUserRoleUseCase:          setRoomUserRoleUseCase,
		ListRoomMembersWithRolesUseCase: listRoomMembersWithRolesUseCase,

		CreateCommunityUseCase:             createCommunityUseCase,
		GetCommunityUseCase:                getCommunityUseCase,
		UpdateCommunityUseCase:             updateCommunityUseCase,
		SearchCommunityUseCase:             searchCommunityUseCase,
		ListMyCommunitiesUseCase:           listMyCommunitiesUseCase,
		ListAllCommunitiesUseCase:          listAllCommunitiesUseCase,
		PromoteToCommunityOwnerUseCase:     promoteToCommunityOwnerUseCase,
		DemoteFromCommunityOwnerUseCase:    demoteFromCommunityOwnerUseCase,
		IsSoleOwnerWithOtherMembersUseCase: isSoleOwnerWithOtherMembersUseCase,

		PubSub: ps,
	}

	// middleware
	e.Use(middleware.RequestLogger())
	e.Use(middleware.Recover())

	// ALLOWED_ORIGINS が設定されていれば本番用ホワイトリスト、未設定なら開発用ワイルドカード
	allowedOrigins := allowedOriginsFromEnv()

	// CORSはJWTより先に登録しないと、401レスポンスにCORSヘッダーが付かずブラウザがブロックする
	e.Use(middleware.CORSWithConfig(middleware.CORSConfig{
		AllowOrigins: allowedOrigins,
		AllowHeaders: []string{
			echo.HeaderOrigin,
			echo.HeaderContentType,
			echo.HeaderAccept,
			echo.HeaderAuthorization,
			"Sec-WebSocket-Protocol",
		},
		AllowMethods: []string{"GET", "POST", "OPTIONS"},
	}))
	// RateLimit はIPベースで安価なため、JWT検証（DB/Redis照合あり）より前に置く
	e.Use(authmiddleware.GraphQLRateLimit())
	e.Use(authmiddleware.JWTAuth(revokedTokenRepository, userRepository))
	e.Use(authmiddleware.GraphQLAudit())
	e.Use(middleware.BodyLimit("21MB")) // メッセージファイル上限 20MB + マージン

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
				return isOriginAllowed(r.Header.Get("Origin"), allowedOrigins)
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
			tokenStr := strings.TrimSpace(strings.TrimPrefix(strings.TrimSpace(authHeader), "Bearer "))
			if tokenStr == "" {
				return nil, nil, fmt.Errorf("missing authorization in websocket init payload")
			}

			claims, err := auth.ValidateAndVerifyToken(ctx, tokenStr, revokedTokenRepository, userRepository)
			if err != nil {
				return nil, nil, err
			}

			ctx = auth.WithClaims(ctx, claims)
			return ctx, nil, nil
		},
	})
	gqlServer.AddTransport(transport.Options{})
	gqlServer.AddTransport(transport.GET{})
	gqlServer.AddTransport(transport.POST{})
	gqlServer.AddTransport(transport.MultipartForm{})
	gqlServer.Use(extension.FixedComplexityLimit(300))

	isProd := os.Getenv("APP_ENV") == "production"
	if !isProd {
		gqlServer.Use(extension.Introspection{})
	}

	// GraphQL エンドポイント (POST: query/mutation, GET: WebSocket subscription)
	gqlHandler := func(c echo.Context) error {
		gqlServer.ServeHTTP(c.Response(), c.Request())
		return nil
	}
	e.POST("/query", gqlHandler)
	e.GET("/query", gqlHandler)

	// Playground（開発環境のみ）
	if !isProd {
		e.GET("/playground", func(c echo.Context) error {
			playground.Handler("GraphQL Playground", "/query").
				ServeHTTP(c.Response(), c.Request())
			return nil
		})
	}

	hub := sse.NewHub()
	// SSE
	e.GET("/events", sse.NewHandler(hub))

	go func() {
		if err := e.Start(":8080"); err != nil && err != http.ErrServerClosed {
			e.Logger.Fatal(err)
		}
	}()

	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit

	logger.Log.Info().Msg("shutting down server...")

	shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	if err := e.Shutdown(shutdownCtx); err != nil {
		logger.Log.Error().Err(err).Msg("server shutdown error")
	}

	if err := database.Close(); err != nil {
		logger.Log.Error().Err(err).Msg("database close error")
	}
	if err := redisClient.Close(); err != nil {
		logger.Log.Error().Err(err).Msg("redis close error")
	}

	logger.Log.Info().Msg("server stopped")
}

// allowedOriginsFromEnv returns the list of allowed CORS/WS origins.
// If ALLOWED_ORIGINS is not set, it falls back to wildcard (dev-friendly).
func allowedOriginsFromEnv() []string {
	raw := strings.TrimSpace(os.Getenv("ALLOWED_ORIGINS"))
	if raw == "" {
		return []string{"*"}
	}
	parts := strings.Split(raw, ",")
	origins := make([]string, 0, len(parts))
	for _, o := range parts {
		if o = strings.TrimSpace(o); o != "" {
			origins = append(origins, o)
		}
	}
	return origins
}

// isOriginAllowed checks whether an origin is in the whitelist.
// A whitelist of ["*"] allows every origin.
func isOriginAllowed(origin string, allowed []string) bool {
	for _, a := range allowed {
		if a == "*" || a == origin {
			return true
		}
	}
	return false
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

	logger.Log.Info().Str("email", email).Msg("initial administrator created")
	return nil
}
