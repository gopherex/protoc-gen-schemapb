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
CONF_PROTO := conformance/v1/conformance.proto conformance/v1/kinds.proto \
  conformance/other/v1/other.proto conformance/edition/v1/edition.proto
# TypeScript only: its names clash in protoc-gen-go itself.
TS_ONLY_PROTO := conformance/names/v1/names.proto

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
	  $(CONF_PROTO) $(TS_ONLY_PROTO) schemapb/options/v1/options.proto schemapb/schema.proto schemapb/value.proto
	$(PROTOCGEN) -plugin "$(BIN)/protoc-gen-ts-schemapb" -out conformance/ts/gen -opt import_extension=js \
	  $(CONF_PROTO) $(TS_ONLY_PROTO)

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

# v2+ needs semantic import versioning (/vN in the module path), unsupported
# here — keep releases on v0/v1.
MAX_MAJOR := 1

.PHONY: release
release: ## Interactive tag + push (vX.Y.Z); go install / easyp deps resolve the tag
	$(MAKE) --no-print-directory _release

# Interactive release: recreate the latest tag on HEAD, or bump major/minor/patch.
_release:
	@cd "$$(git rev-parse --show-toplevel)"
	if [ -n "$$(git status --porcelain)" ]; then
	  echo "✗ Working tree not clean — commit or stash first:"
	  git status --short
	  exit 1
	fi
	cur="$$(git tag -l 'v[0-9]*.[0-9]*.[0-9]*' | sed 's/^v//' | sort -t. -k1,1n -k2,2n -k3,3n | tail -1)"
	cur="$${cur:-0.0.0}"
	head="$$(git rev-parse --short HEAD)"
	echo "Latest release: v$$cur    HEAD: $$head"
	echo
	echo "  1) recreate v$$cur on HEAD   [force]"
	echo "  2) bump version"
	echo "  3) cancel"
	read -r -p "> " action
	case "$$action" in
	1)
	  if ! git tag -l "v$$cur" | grep -q .; then echo "✗ No release tag to recreate."; exit 1; fi
	  echo "Will DELETE and recreate v$$cur on $$head, then force-push."
	  read -r -p "Type 'yes' to proceed: " ok
	  [ "$$ok" = "yes" ] || { echo "Aborted."; exit 0; }
	  git tag -d "v$$cur" 2>/dev/null || true
	  git push origin ":refs/tags/v$$cur" 2>/dev/null || true
	  git tag -a "v$$cur" -m "v$$cur"
	  git push origin --force "v$$cur"
	  echo "✓ Recreated v$$cur on $$head."
	  ;;
	2)
	  IFS=. read -r MA MI PA <<< "$$cur"
	  echo
	  echo "  1) major  -> v$$((MA+1)).0.0"
	  echo "  2) minor  -> v$$MA.$$((MI+1)).0"
	  echo "  3) patch  -> v$$MA.$$MI.$$((PA+1))"
	  read -r -p "> " comp
	  case "$$comp" in
	    1) MA=$$((MA+1)); MI=0; PA=0 ;;
	    2) MI=$$((MI+1)); PA=0 ;;
	    3) PA=$$((PA+1)) ;;
	    *) echo "Aborted."; exit 0 ;;
	  esac
	  if [ "$$MA" -gt "$(MAX_MAJOR)" ]; then
	    echo "✗ v$$MA needs semantic import versioning (/v$$MA in the module path); stay on v0/v1."
	    exit 1
	  fi
	  new="$$MA.$$MI.$$PA"
	  echo
	  echo "Release v$$new — create tag v$$new on $$head and push."
	  read -r -p "Type 'yes' to proceed: " ok
	  [ "$$ok" = "yes" ] || { echo "Aborted."; exit 0; }
	  git tag -a "v$$new" -m "v$$new"
	  git push origin "v$$new"
	  echo "✓ Released v$$new — go install / easyp deps resolve the tag."
	  ;;
	*)
	  echo "Cancelled."
	  ;;
	esac

.PHONY: clean
clean: ## Remove tools and build output
	rm -rf "$(BIN)" "$(TOOLS)" "$(BUILD)" conformance/ts/node_modules
