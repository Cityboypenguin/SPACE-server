package main

import (
	"log"
	"net/http"
	"os"

	"github.com/99designs/gqlgen/graphql/handler"
	"github.com/99designs/gqlgen/graphql/handler/extension"
	"github.com/99designs/gqlgen/graphql/handler/lru"
	"github.com/99designs/gqlgen/graphql/handler/transport"
	"github.com/99designs/gqlgen/graphql/playground"
	"github.com/Cityboypenguin/SPACE-server/graph"
	"github.com/vektah/gqlparser/v2/ast"
	"github.com/Cityboypenguin/SPACE-server/infra/inmem"
	postUsecases "github.com/Cityboypenguin/SPACE-server/usecase/post"
	userUsecases "github.com/Cityboypenguin/SPACE-server/usecase/user"
)

const defaultPort = "8080"

func main() {
	port := os.Getenv("PORT")
	if port == "" {
		port = defaultPort
	}

	// Initialize repositories
	userRepo := inmem.NewUserRepository()
	postRepo := inmem.NewPostRepository()

	// Initialize usecases
	resolver := &graph.Resolver{
		SignUpUseCase:     &userUsecases.SignUpInteractor{UserRepository: userRepo},
		GetUserUseCase:    &userUsecases.GetUserInteractor{UserRepository: userRepo},
		GetUsersUseCase:   &userUsecases.GetUsersInteractor{UserRepository: userRepo},
		CreatePostUseCase: &postUsecases.CreatePostInteractor{PostRepository: postRepo},
		GetPostUseCase:    &postUsecases.GetPostInteractor{PostRepository: postRepo},
		GetPostsUseCase:   &postUsecases.GetPostsInteractor{PostRepository: postRepo},
	}

	srv := handler.New(graph.NewExecutableSchema(graph.Config{Resolvers: resolver}))

	srv.AddTransport(transport.Options{})
	srv.AddTransport(transport.GET{})
	srv.AddTransport(transport.POST{})

	srv.SetQueryCache(lru.New[*ast.QueryDocument](1000))

	srv.Use(extension.Introspection{})
	srv.Use(extension.AutomaticPersistedQuery{
		Cache: lru.New[string](100),
	})

	http.Handle("/", playground.Handler("GraphQL playground", "/query"))
	http.Handle("/query", srv)

	log.Printf("connect to http://localhost:%s/ for GraphQL playground", port)
	log.Fatal(http.ListenAndServe(":"+port, nil))
}
