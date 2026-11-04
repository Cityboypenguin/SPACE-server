package dataloader

import (
	"context"
	"testing"

	"github.com/99designs/gqlgen/graphql"
	"github.com/vektah/gqlparser/v2/ast"
)

func TestFreshLoadersForSubscriptions(t *testing.T) {
	original := &Loaders{}
	withOperation := func(op ast.Operation) context.Context {
		ctx := WithLoaders(context.Background(), original)
		return graphql.WithOperationContext(ctx, &graphql.OperationContext{Operation: &ast.OperationDefinition{Operation: op}})
	}
	created := 0
	middleware := FreshLoadersForSubscriptions(func() *Loaders {
		created++
		return &Loaders{}
	})
	seen := []*Loaders{}
	next := func(ctx context.Context) *graphql.Response {
		seen = append(seen, For(ctx))
		return &graphql.Response{}
	}

	// 配信のたびに別のローダーになる。
	ctx := withOperation(ast.Subscription)
	middleware(ctx, next)
	middleware(ctx, next)
	if created != 2 || seen[0] == original || seen[0] == seen[1] {
		t.Fatalf("subscription events must not share loaders: created=%d", created)
	}

	// 通常のリクエストではバッチのために同じローダーのまま。
	middleware(withOperation(ast.Query), next)
	if created != 2 || seen[2] != original {
		t.Errorf("queries must keep the request's loaders: created=%d", created)
	}
}
