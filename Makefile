# protoc-gen-schemapb — schemapb validators generated from .proto options.
#
# Conventions (as in schemapb):
#   - ALL tools live in ./bin (version-pinned below); nothing is global.
#     `git clone` + `make configure` = fully working environment.
#   - schemapb is consumed as released: the Go module (go.mod), the contract
#     protos (easyp deps) and the TS runtime (@gopherex/schemapb on GitHub
#     Packages, read with GITHUB_TOKEN).
#   - protos are compiled once by easyp into a FileDescriptorSet; every
#     plugin (protoc-gen-go, protoc-gen-es, ours) runs over it through
#     internal/tools/protocgen. No protoc, no registry.
#   - `make help` lists every target.
#
# Prerequisites (system): go, node/npm.

SHELL := /bin/bash
.ONESHELL:
.SHELLFLAGS := -eu -o pipefail -c
.DEFAULT_GOAL := help

BIN   := $(CURDIR)/bin
TOOLS := $(CURDIR)/.tools
BUILD := $(CURDIR)/.build

# ---- pinned tool versions (bumps are explicit commits) ----------------------
EASYP_VERSION          := v0.16.6
PROTOC_GEN_GO_VERSION  := v1.36.11
GOLANGCI_LINT_VERSION  := v2.6.2
YARN_BOOTSTRAP_VERSION := 1.22.22

EASYP     := "$(BIN)/easyp"
YARN      := "$(BIN)/yarn"
DESC      := $(BUILD)/descriptors.binpb
PROTOCGEN := go run ./internal/tools/protocgen -set $(DESC)
CONF_PROTO := conformance/v1/conformance.proto

.PHONY: help
help: ## List all targets with explanations
	@grep -hE '^[a-zA-Z0-9_.-]+:.*## ' $(MAKEFILE_LIST) | \
	  awk -F':.*## ' '{printf "  \033[36m%-14s\033[0m %s\n", $$1, $$2}'

.PHONY: configure
configure: ## Fetch all pinned tools into ./bin and install the TS harness
	mkdir -p "$(BIN)" "$(TOOLS)"
	echo "--- easyp $(EASYP_VERSION)"
	GOBIN="$(BIN)" go install github.com/easyp-tech/easyp/cmd/easyp@$(EASYP_VERSION)
	echo "--- protoc-gen-go $(PROTOC_GEN_GO_VERSION)"
	GOBIN="$(BIN)" go install google.golang.org/protobuf/cmd/protoc-gen-go@$(PROTOC_GEN_GO_VERSION)
	echo "--- golangci-lint $(GOLANGCI_LINT_VERSION)"
	GOBIN="$(BIN)" go install github.com/golangci/golangci-lint/v2/cmd/golangci-lint@$(GOLANGCI_LINT_VERSION)
	echo "--- yarn bootstrap $(YARN_BOOTSTRAP_VERSION) (conformance/ts pins its own via yarnPath)"
	mkdir -p "$(TOOLS)/npm-yarn"
	npm install --prefix "$(TOOLS)/npm-yarn" --no-save --silent yarn@$(YARN_BOOTSTRAP_VERSION)
	ln -sf "$(TOOLS)/npm-yarn/node_modules/.bin/yarn" "$(BIN)/yarn"
	cd conformance/ts && $(YARN) install --immutable
	echo "✓ configure done — tools in $(BIN)"

.PHONY: build
build: ## Build bin/protoc-gen-go-schemapb and bin/protoc-gen-ts-schemapb
	go build -o "$(BIN)/protoc-gen-go-schemapb" ./cmd/protoc-gen-go-schemapb
	go build -o "$(BIN)/protoc-gen-ts-schemapb" ./cmd/protoc-gen-ts-schemapb

.PHONY: descriptors
descriptors: ## Compile every proto into .build/descriptors.binpb
	mkdir -p "$(BUILD)"
	$(EASYP) mod download
	$(EASYP) generate --descriptor_set_out "$(DESC)" --include_imports

.PHONY: gen
gen: build descriptors ## Regenerate options Go code and the conformance code
	$(PROTOCGEN) -plugin "$(BIN)/protoc-gen-go" -out . -opt paths=source_relative \
	  schemapb/options/v1/options.proto
	rm -rf conformance/gen/go conformance/ts/gen
	$(PROTOCGEN) -plugin "$(BIN)/protoc-gen-go" -out conformance/gen/go -opt paths=source_relative $(CONF_PROTO)
	$(PROTOCGEN) -plugin "$(BIN)/protoc-gen-go-schemapb" -out conformance/gen/go -opt paths=source_relative $(CONF_PROTO)
	$(PROTOCGEN) -plugin conformance/ts/node_modules/.bin/protoc-gen-es -out conformance/ts/gen \
	  -opt target=ts,import_extension=js \
	  $(CONF_PROTO) schemapb/options/v1/options.proto schemapb/schema.proto schemapb/value.proto
	$(PROTOCGEN) -plugin "$(BIN)/protoc-gen-ts-schemapb" -out conformance/ts/gen -opt import_extension=js $(CONF_PROTO)

.PHONY: golden
golden: ## Rewrite the conformance goldens from the Go backend (reference)
	go test ./conformance/go -update

.PHONY: test
test: test-go test-ts ## Run all tests

.PHONY: test-go
test-go: ## Go tests: frontend, backends, Go conformance
	go test ./...

.PHONY: test-ts
test-ts: ## TS conformance against the released @gopherex/schemapb runtime
	cd conformance/ts && $(YARN) typecheck && $(YARN) test

.PHONY: lint
lint: ## Lint protos (easyp) and Go code (golangci-lint)
	$(EASYP) lint -p schemapb
	"$(BIN)/golangci-lint" run ./...

.PHONY: clean
clean: ## Remove tools and build output
	rm -rf "$(BIN)" "$(TOOLS)" "$(BUILD)" conformance/ts/node_modules
