.PHONY: generate check-generated

GO ?= go

generate:
	$(GO) tool gqlgen generate

check-generated:
	@set -e; \
	tmp=$$(mktemp -d); \
	cp graph/generated.go graph/model/models_gen.go graph/schema.resolvers.go $$tmp/; \
	trap 'rm -rf $$tmp' EXIT; \
	if ! $(GO) tool gqlgen generate; then \
		cp $$tmp/generated.go graph/generated.go; \
		cp $$tmp/models_gen.go graph/model/models_gen.go; \
		cp $$tmp/schema.resolvers.go graph/schema.resolvers.go; \
		echo "gqlgen generation failed; restored all generated files" >&2; \
		exit 1; \
	fi; \
	if ! cmp -s $$tmp/schema.resolvers.go graph/schema.resolvers.go; then \
		cp $$tmp/generated.go graph/generated.go; \
		cp $$tmp/models_gen.go graph/model/models_gen.go; \
		cp $$tmp/schema.resolvers.go graph/schema.resolvers.go; \
		echo "gqlgen modified graph/schema.resolvers.go; restored all generated files" >&2; \
		exit 1; \
	fi
	git diff --exit-code -- graph/generated.go graph/model/models_gen.go
