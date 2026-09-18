.PHONY: generate check-generated

generate:
	go tool gqlgen generate

check-generated:
	@tmp=$$(mktemp); \
	cp graph/schema.resolvers.go $$tmp; \
	trap 'rm -f $$tmp' EXIT; \
	go tool gqlgen generate; \
	if ! cmp -s $$tmp graph/schema.resolvers.go; then \
		cp $$tmp graph/schema.resolvers.go; \
		echo "gqlgen modified graph/schema.resolvers.go; restored the original resolver" >&2; \
		exit 1; \
	fi
	git diff --exit-code -- graph/generated.go graph/model/models_gen.go graph/schema.resolvers.go
