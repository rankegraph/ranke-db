# ranke-db — repo-level targets.
#
# The OpenAPI spec is the single source of truth; `make generate` produces every
# artifact from it into openapi/ (alongside the spec) — the Go server, the TS/JS
# client, and the HTML + Markdown references — and symlinks the two references
# under docs/openapi/.

OPENAPI   := openapi/openapi.yaml
API_OUT   := openapi
# The explorer reads an instance through this client, and `frontend/` builds on its own
# (its Makefile and package.json are not wired into this one). So the generated client is
# copied in and committed, as `frontend/dist/explorer.html` is: `make -C frontend` needs
# nothing from here, and `check-generated` notices when the copy goes stale.
EXPLORER_CLIENT := frontend/src/core/data/openapi.gen.ts
# openapi.yaml $refs rql.schema.json, which no generator resolves on its own, so
# every one of them reads this bundle: the same document with the external schema
# lifted into components/schemas.
OPENAPI_GEN := openapi/openapi.gen.yaml

# The node generators, pinned exactly. A floating `latest` or major range lets an
# upstream release rewrite the artifacts, which trips check-generated for a reason
# that is not ours. Bump deliberately, then regenerate.
REDOCLY     := @redocly/cli@2.44.1
SWAGGER_TS  := swagger-typescript-api@13.12.6
WIDDERSHINS := widdershins@4.0.1
RANKE_GO_MOD ?= github.com/rankegraph/ranke-go
# The library's other half. frontend/ keeps its own report, but nothing invokes it, so a
# stale pin sat unseen for eleven days while the two halves disagreed about the wire format.
# The pin is read straight out of package.json rather than delegated to that target, which
# would drag node_modules into `verify` and wire frontend/ into a build it stays out of.
RANKE_TS_PKG ?= @rankegraph/ranke
FRONTEND_PKG := frontend/package.json
RANKE_GO_VERSION ?= latest
# ask = prompt before raising the go directive; keep = leave it; or a version.
GO_VERSION ?= ask

RANKE_GRAPH_REPO    ?= https://github.com/rankegraph/ranke-graph
RANKE_GRAPH_REF     ?= main
# The ranke-graph release docs-release traces its documents to — bumped
# deliberately, the same discipline RANKE_GO_VERSION's pin holds ranke-go to,
# never "latest": a floating pin would make `make docs-release` non-reproducible
# between two runs that happen to straddle a ranke-graph release.
RANKE_GRAPH_RELEASE ?= v0.23.3
PAPERS_DIR          := docs/papers
DOCS_DIR            := docs
DIST_DIR            := dist
DOCS_PDF            := $(DIST_DIR)/docs.pdf
# What this repo publishes for a consumer to read or render itself — the chapters
# and the two machine-readable contracts. The website and any client generator
# take one tarball per repo instead of cloning four.
DOCS_BUNDLE_NAME := ranke-db-docs
DOCS_BUNDLE      := $(DIST_DIR)/$(DOCS_BUNDLE_NAME).tar.gz
# "dev" locally; a release build overrides it so the printed handbook names its tag
# rather than saying "dev" (-> shared/handbook.typ's own version input).
DOCS_VERSION     ?= dev

# fetch-ranke-docs.sh lives in ranke-graph and serves every consumer repo, so it is
# downloaded rather than vendored — a change to how documents are fetched is made
# once, upstream. Cached under bin/ (gitignored) like brokkr below.
RANKE_FETCHER     := bin/fetch-ranke-docs.sh
# Bare $(RANKE_GRAPH_REF), not refs/heads/$(RANKE_GRAPH_REF): raw.githubusercontent.com
# resolves a branch or a tag alike here, and refs/heads/ would 404 were this ever
# pointed at a tag — matching the canonical form fetch-ranke-docs.sh's own header
# documents.
RANKE_FETCHER_URL ?= https://raw.githubusercontent.com/rankegraph/ranke-graph/$(RANKE_GRAPH_REF)/scripts/fetch-ranke-docs.sh

# release-cycle.sh, same reasoning: the git mechanics of a release (branch
# resolution, the merge-then-tag dance, the wait for CI) belong to ranke-graph
# too, so this repo carries no fork of them. What differs here from another
# consumer is nothing — no scripts/release-next-version.sh, -pretag.sh, or
# -feature-branch-only — so this repo's release is exactly the shared cycle.
RELEASE_CYCLER     := bin/release-cycle.sh
RELEASE_CYCLER_URL ?= https://raw.githubusercontent.com/rankegraph/ranke-graph/$(RANKE_GRAPH_REF)/scripts/release-cycle.sh

# The typst release ranke-graph's own release.yml builds its papers with, installed
# exactly by the release job so a published handbook is one ranke-graph would have
# built. check-tools and docs-pdf hold a local toolchain to the SERIES: typst is
# pre-1.0, so the minor is what changes a layout and the patch is a bug fix.
TYPST         := typst
TYPST_VERSION := 0.15.0
TYPST_SERIES  := $(basename $(TYPST_VERSION))
TYPST_URL     := https://github.com/typst/typst/releases/tag/v$(TYPST_VERSION)

# The oldest Node the generation tools run on, checked by check-tools rather than
# only named in its install hint.
NODE_MIN      := 18

# brokkr, installed on demand rather than assumed present. Cached under bin/ (already
# gitignored, already this repo's build-output directory) — the installer itself checks
# the latest release against what is already there and only downloads on a mismatch.
TOOLS_BIN         := bin/tools
BROKKR            := $(TOOLS_BIN)/brokkr
BROKKR_INSTALL_SH := https://raw.githubusercontent.com/flocko-motion/sindri/master/scripts/install-brokkr.sh

.PHONY: all help check check-tools generate verify lint tidy build smoke test dev seed \
        ranke-go-version upgrade release major minor patch breaking feature fix \
        docs docs-papers docs-current docs-release docs-check docs-pdf docs-bundle docs-clean print-typst-version \
        pull-rql-schema check-rql-schema check-generated release-gate check-clean-tree check-release-bump

BIN := bin/ranke-db
GEN := bin/generator

.DEFAULT_GOAL := all

all: generate verify ## Default: regenerate from the spec, then build/vet/test

help: ## Show this help
	@grep -hE '^[a-zA-Z_-]+:.*?## ' $(MAKEFILE_LIST) \
		| awk 'BEGIN{FS=":.*?## "}{printf "  \033[36m%-14s\033[0m %s\n", $$1, $$2}'

# --- The RQL schema, pulled from ranke-graph -------------------------------

# openapi.yaml $refs this file for the whole read language, so the query type is
# ranke-graph's and this repo holds no second copy of it. Committed, which keeps
# `make generate` offline and reproducible; run pull-rql-schema to take a newer
# revision and review the diff.
#
# Taken from the spec on `main`, not from the latest release: the specification is the
# source of truth, and an implementation that disagrees with it has the bug. A release is
# a snapshot of that source, so vendoring one means serving whatever the spec said when it
# was cut — which is how this contract came to require `output.content.overflow` for a day
# after R-QCONTENT made it optional. Point RQL_SCHEMA_URL at a release to pin one instead.
RQL_SCHEMA     := $(API_OUT)/rql.schema.json
RQL_SCHEMA_URL ?= https://raw.githubusercontent.com/rankegraph/ranke-graph/refs/heads/$(RANKE_GRAPH_REF)/spec/rql.schema.json

pull-rql-schema: ## Pull rql.schema.json from the ranke-graph spec into openapi/
	@command -v curl > /dev/null || { echo "curl not found"; exit 1; }
	@command -v jq > /dev/null || { echo "jq not found"; exit 1; }
	@echo ">> pull     → $(RQL_SCHEMA)  ($(RQL_SCHEMA_URL))"
	@tmp=$$(mktemp); \
	curl -fsSL "$(RQL_SCHEMA_URL)" -o "$$tmp" || { rm -f "$$tmp"; echo "download failed"; exit 1; }; \
	jq -e 'has("$$defs") and (.["$$defs"] | has("Query"))' "$$tmp" > /dev/null \
		|| { rm -f "$$tmp"; echo "downloaded file is not the RQL schema"; exit 1; }; \
	mv "$$tmp" $(RQL_SCHEMA); \
	chmod 644 $(RQL_SCHEMA)
	@git diff --stat -- $(RQL_SCHEMA)

# --- Code/doc generation from the OpenAPI spec -----------------------------

check-tools: ## Verify the toolchain is installed at the versions this repo pins (reports all missing at once)
	@missing=0; \
	check() { command -v "$$1" >/dev/null 2>&1 || { printf "  missing: %-6s → %s\n" "$$1" "$$2"; missing=1; }; }; \
	check go  "https://go.dev/dl/"; \
	check npx "Node.js $(NODE_MIN)+ — https://nodejs.org (provides npx; the TS client and API docs run via npx)"; \
	check $(TYPST) "$(TYPST_URL) (builds the handbook; pin v$(TYPST_VERSION))"; \
	if [ "$$missing" -ne 0 ]; then \
		echo "ERROR: install the tool(s) above, then re-run. (npx fetches swagger-typescript-api, @redocly/cli, and widdershins on first use.)"; exit 1; \
	fi; \
	major=$$(node -v 2>/dev/null | sed 's/^v//; s/\..*//'); \
	case "$$major" in ''|*[!0-9]*) major= ;; esac; \
	if [ -n "$$major" ] && [ "$$major" -lt $(NODE_MIN) ]; then \
		echo "  node $$(node -v), below the $(NODE_MIN)+ the generation tools need → https://nodejs.org"; exit 1; \
	fi; \
	have=$$($(TYPST) --version 2>/dev/null | awk 'NR==1 {print $$2}'); have=$${have:-unknown}; \
	if [ "$$(echo "$$have" | cut -d. -f1,2)" != "$(TYPST_SERIES)" ]; then \
		echo "  typst $$have, but the handbook is built with the $(TYPST_SERIES) series → $(TYPST_URL)"; \
		echo "ERROR: typst series mismatch — a minor release lays the page out its own way, so only $(TYPST_SERIES) reproduces ranke-graph's handbook (its releases install $(TYPST_VERSION))."; exit 1; \
	fi; \
	echo "toolchain OK (go + node + typst $$have)"

generate: check-tools ## Generate every artifact from the spec into openapi/ (Go server, TS client, HTML, Markdown) + the explorer's copy of the client + docs/openapi/ symlinks
	@echo ">> bundle   → $(OPENAPI_GEN)"
	@npx --yes $(REDOCLY) bundle $(OPENAPI) -o $(OPENAPI_GEN) >/dev/null
	@echo ">> gen-go   → $(API_OUT)/openapi.gen.go"
	@go tool oapi-codegen -config $(API_OUT)/oapi-codegen.yaml $(OPENAPI_GEN)
	@echo ">> gen-ts   → $(API_OUT)/openapi.gen.ts"
	@npx --yes $(SWAGGER_TS) generate -p $(OPENAPI_GEN) -o $(API_OUT) -n openapi.gen.ts >/dev/null
	@echo ">> copy     → $(EXPLORER_CLIENT)"
	@cp $(API_OUT)/openapi.gen.ts $(EXPLORER_CLIENT)
	@echo ">> gen-html → $(API_OUT)/openapi.html"
	@npx --yes $(REDOCLY) build-docs $(OPENAPI_GEN) -o $(API_OUT)/openapi.html >/dev/null
	@echo ">> gen-md   → $(API_OUT)/openapi.md"
	@npx --yes $(WIDDERSHINS) $(OPENAPI_GEN) -o $(API_OUT)/openapi.md --summary --code >/dev/null
	@echo ">> link     → docs/openapi/openapi.{html,md}"
	@mkdir -p docs/openapi
	@ln -sf ../../$(API_OUT)/openapi.html docs/openapi/openapi.html
	@ln -sf ../../$(API_OUT)/openapi.md docs/openapi/openapi.md

# --- Build / test ----------------------------------------------------------

tidy: ## Sync go.mod/go.sum with imports (adds transitive deps)
	@go mod tidy

build: ## Compile both binaries into bin/ (the server and the seeding client)
	@echo ">> build → $(BIN)"
	@go build -o $(BIN) ./cmd/ranke-db
	@echo ">> build → $(GEN)"
	@go build -o $(GEN) ./cmd/generator

DEV_CONFIG ?= examples/minimal/config.json
SEED_URL   ?= http://localhost:8080
# Shape knobs for SEED=chain; ignored by the example shape.
CONTRIBUTIONS ?= 20
CLAIMS        ?= 10

# SEED picks a shape: example (a handful of claims over three branches), release (the release
# process the slides draw — four signing identities, two packages meeting at one artifact),
# chain (as many contributions as CONTRIBUTIONS × CLAIMS asks for), or big — a chain sized past
# the explorer's lens threshold, so the timeline's stretching is exercised on a graph that
# needs it.
BIG_CONTRIBUTIONS ?= 1200
BIG_CLAIMS        ?= 20
RELEASES          ?= 3

SEED_ARGS = $(strip $(if $(filter big,$(SEED)), \
	chain --contributions $(BIG_CONTRIBUTIONS) --claims $(BIG_CLAIMS), \
	$(if $(filter chain,$(SEED)),chain --contributions $(CONTRIBUTIONS) --claims $(CLAIMS), \
	$(if $(filter release,$(SEED)),release --releases $(RELEASES),example))))

# The signing key is minted per run and never written down: this stack keeps
# nothing between launches, so a throwaway identity is the honest default. The
# address comes from the config's endpoints section, not a flag.
#
# Seeding runs as a client, which is what a contributor is — so the generator goes into
# the background with --wait, contributes as soon as /health answers, and exits, while
# the server keeps the foreground and ctrl-c.
# -tags explorer here, not in `build`: a dev loop is exactly where clicking through
# /explorer is wanted, and dist/explorer.html is committed, so this needs no frontend
# build step. `build`/`smoke`/CI stay untagged — that default is unrelated to this one.
dev: ## Run a dev server from DEV_CONFIG with /explorer active (SEED=example|release|chain|big to seed it once it answers)
	@command -v openssl >/dev/null 2>&1 || { echo "ERROR: dev needs openssl to mint a throwaway signing key"; exit 1; }
	@echo ">> build → $(BIN) (-tags explorer)"
	@go build -tags explorer -o $(BIN) ./cmd/ranke-db
	@echo ">> build → $(GEN)"
	@go build -o $(GEN) ./cmd/generator
	@addr=$$(grep -o '"addr"[[:space:]]*:[[:space:]]*"[^"]*"' $(DEV_CONFIG) | head -1 | sed -E 's/.*"([^"]*)"$$/\1/'); \
		addr=$${addr:-:8080}; port=$${addr#:}; url="http://localhost$$addr"; \
		if command -v lsof >/dev/null 2>&1; then \
			pid=$$(lsof -ti tcp:"$$port" 2>/dev/null | head -1); \
			if [ -n "$$pid" ]; then \
				name=$$(ps -p "$$pid" -o comm= 2>/dev/null || echo "?"); \
				printf ">> port %s is already in use (pid %s, %s) — kill it and continue? [y/N] " "$$port" "$$pid" "$$name"; \
				read -r ans; \
				case "$$ans" in \
					y|Y) kill "$$pid"; sleep 0.3 ;; \
					*) echo "aborting — free the port yourself, or point DEV_CONFIG at a different addr"; exit 1 ;; \
				esac; \
			fi; \
		fi; \
		mkdir -p -m 0700 run; \
		echo ">> $(DEV_CONFIG) — ephemeral signing key, nothing persisted between runs"; \
		echo ">> serving on  $$url"; \
		echo ">> try:  curl $$url/health  ·  curl $$url/branches  ·  curl $$url/branches/main/head  ·  open $$url/explorer"; \
		echo ">> ctrl-c to stop"; \
		$(if $(SEED),$(GEN) $(SEED_ARGS) "$$url" --wait 15s &,) \
		RANKE_SIGNER_KEY="$$(openssl genpkey -algorithm ed25519)" $(BIN) run --dev $(DEV_CONFIG)

# Seeding a server that is already up. An in-memory stack dies with its process, so
# against the default config this only reaches an instance started by `make dev`.
seed: build ## Seed a running server over its REST API (SEED_URL, SEED=example|release|chain|big)
	@$(GEN) $(SEED_ARGS) $(SEED_URL) --wait 5s

smoke: build ## Launch against the minimal example, seed it over the API, read it back, shut down
	@./scripts/smoke.sh $(BIN) $(GEN)

test: ## Test all packages; scope with make test/<pkg> (e.g. test/config, test/adapters/vault/openbao)
	@echo ">> go test ./..."
	@go test ./...

# make test/<pkg-path> runs one package subtree, verbose — e.g. make test/adapters
# (all adapters), make test/adapters/vault/openbao (one impl), make test/config.
# One pattern rule routes any repo-relative path ($* captures the rest).
test/%:
	@echo ">> go test -v ./$*/..."
	@go test -v ./$*/...

verify: generate docs-pdf ## Regenerate from the spec, then build, vet, test, gofmt-check, lint, and build the handbook
	@set -e; \
		go build ./...; \
		go vet ./...; \
		fmt=$$(gofmt -l $$(git ls-files '*.go' | grep -v '\.gen\.go$$')); \
		[ -z "$$fmt" ] || { echo "gofmt needed:"; echo "$$fmt"; exit 1; }; \
		go test ./...; \
		$(MAKE) --no-print-directory lint
	@$(MAKE) -s ranke-go-version
	@$(MAKE) -s ranke-ts-version

lint: ## Run brokkr lint — one already on PATH if there is one, else this repo's cached copy in bin/tools/ (the installer checks GitHub for a newer release every run, skipping the download itself when already current)
	@if command -v brokkr >/dev/null 2>&1; then bin=brokkr; \
	else \
		command -v curl >/dev/null 2>&1 || { echo "ERROR: brokkr not found and curl is not on PATH to install it"; exit 1; }; \
		curl -fsSL $(BROKKR_INSTALL_SH) | bash -s -- $(BROKKR); \
		bin=$(BROKKR); \
	fi; \
	"$$bin" lint

# `verify` is the Go half's own gate; frontend/ is deliberately not wired into this
# Makefile's build (its own Makefile builds it standalone). `check` reaches across that
# boundary only to run both halves' gates in one command — it adds nothing to either
# side's own build, so frontend/ still builds and verifies on its own exactly as before.
check: verify ## Whole-repo quality gate: verify (Go), then frontend/'s own check + test
	@$(MAKE) -C frontend check
	@$(MAKE) -C frontend test

# $(RANKE_FETCHER)/$(RELEASE_CYCLER) are file targets with no prerequisite, so once
# cached under bin/ they are never re-fetched on their own — a stale copy (missing a
# ranke-graph fix, or a whole new document directory) would sit there forever
# otherwise. upgrade is the one command that already means "bring everything to
# latest", so refreshing them here is what makes that true rather than aspirational.
upgrade: ## Upgrade all deps, tools and ranke-go to latest, tidy, then verify; asks before raising the go directive (GO_VERSION=keep|1.26.5, RANKE_GO_VERSION=vX.Y.Z)
	@GO_VERSION=$(GO_VERSION) \
		RANKE_GO_MOD=$(RANKE_GO_MOD) \
		RANKE_GO_VERSION=$(RANKE_GO_VERSION) \
		./scripts/upgrade.sh
	@rm -f $(RANKE_FETCHER) $(RELEASE_CYCLER)
	@$(MAKE) $(RANKE_FETCHER) $(RELEASE_CYCLER)

ranke-ts-version: ## Recommend a ranke-ts bump if a newer release exists
	-@[ -f $(FRONTEND_PKG) ] && { \
		cur=$$(sed -n 's|.*"$(RANKE_TS_PKG)": *"[^0-9]*\([^"]*\)".*|\1|p' $(FRONTEND_PKG)); \
		latest=$$(npm view $(RANKE_TS_PKG) version 2>/dev/null); \
		if [ -n "$$cur" ] && [ -n "$$latest" ] && [ "$$cur" != "$$latest" ]; then \
			echo ">> ranke-ts: on $$cur, latest is $$latest — bump: make -C frontend upgrade"; \
		fi; \
	} || true

ranke-go-version: ## Recommend a ranke-go bump if a newer release exists
	-@grep -q "$(RANKE_GO_MOD)" go.mod 2>/dev/null && { \
		cur=$$(go list -m -f '{{.Version}}' $(RANKE_GO_MOD) 2>/dev/null); \
		latest=$$(go list -m -f '{{.Version}}' $(RANKE_GO_MOD)@latest 2>/dev/null); \
		if [ -n "$$latest" ] && [ "$$cur" != "$$latest" ]; then \
			echo ">> ranke-go: on $$cur, latest is $$latest — bump: go get $(RANKE_GO_MOD)@latest"; \
		fi; \
	} || true

# --- Release / papers (unchanged) ------------------------------------------

# --- Release gate ----------------------------------------------------------
#
# The artifacts can lie in two silent ways: the RQL schema moved in ranke-graph, or
# someone edited the spec here and never regenerated. Either ships a
# contract the code does not implement, so releasing checks both and refuses.

check-rql-schema: ## Fail if the vendored RQL schema differs from the ranke-graph spec
	@command -v curl > /dev/null || { echo "curl not found"; exit 1; }
	@echo ">> check    → $(RQL_SCHEMA) against the spec"
	@tmp=$$(mktemp); \
	curl -fsSL "$(RQL_SCHEMA_URL)" -o "$$tmp" \
		|| { rm -f "$$tmp"; echo "   cannot reach $(RQL_SCHEMA_URL)"; exit 1; }; \
	if diff -q "$$tmp" $(RQL_SCHEMA) > /dev/null 2>&1; then \
		rm -f "$$tmp"; echo "   matches the spec"; \
	else \
		echo ""; diff -u $(RQL_SCHEMA) "$$tmp" | head -40; rm -f "$$tmp"; \
		echo ""; \
		echo "   the RQL schema moved in ranke-graph — the source of truth is ahead of this copy."; \
		echo "   Take it:  make pull-rql-schema && make generate"; \
		exit 1; \
	fi

# Asks whether regenerating changes anything, rather than whether the tree matches
# HEAD — so uncommitted work in hand is not mistaken for drift. `generate` is
# byte-idempotent, so a changed hash means the artifacts were stale.
GEN_ARTIFACTS := $(OPENAPI_GEN) $(API_OUT)/openapi.gen.go $(API_OUT)/openapi.gen.ts \
                 $(API_OUT)/openapi.html $(API_OUT)/openapi.md $(EXPLORER_CLIENT)

check-generated: ## Fail if `make generate` would change anything (spec edited without regenerating); auto-commits the rebuild
	@sums=$$(mktemp); \
	md5sum $(GEN_ARTIFACTS) > "$$sums" 2>/dev/null || true; \
	$(MAKE) --no-print-directory generate; \
	if md5sum --status -c "$$sums" 2>/dev/null; then \
		rm -f "$$sums"; echo ">> check    → generated artifacts are current"; \
	else \
		rm -f "$$sums"; echo ""; \
		echo "   the artifacts were stale — the spec changed without a regenerate."; \
		echo "   Committing the rebuild (only these paths, nothing else in the tree):"; \
		git status --short -- $(GEN_ARTIFACTS) | sed 's/^/     /'; \
		git commit --quiet -m "chore: regenerate stale OpenAPI artifacts" -- $(GEN_ARTIFACTS); \
		echo "   committed — re-run to confirm the tree is now current."; \
		exit 1; \
	fi

release-gate: check-rql-schema check-generated ## Run the pre-release contract checks without releasing

# check-clean-tree first, ahead of release-gate: a dirty tree is a free, instant
# check, and release-gate is not — failing on it should not cost a build first.
check-clean-tree:
	@[ -z "$$(git status --porcelain)" ] || { echo "working tree is dirty — commit or stash before releasing" >&2; exit 1; }

# Same reasoning as check-clean-tree: a missing or misspelled bump word is a free,
# instant check, and release-gate is not — release-cycle.sh's own case statement
# still validates it too, but only after release-gate already ran.
check-release-bump:
	@[ -n "$(filter major minor patch breaking feature fix,$(MAKECMDGOALS))" ] || \
		{ echo "usage: make release <major|breaking | minor|feature | patch|fix>" >&2; exit 1; }

release: check-clean-tree check-release-bump release-gate $(RELEASE_CYCLER) ## Release: clean → merge to default via PR → tag merged tip → push (bump: major|minor|patch, aliases breaking|feature|fix)
	@$(RELEASE_CYCLER) $(filter major minor patch breaking feature fix,$(MAKECMDGOALS))

major minor patch breaking feature fix:
	@:

$(RANKE_FETCHER): ## Cache fetch-ranke-docs.sh from ranke-graph (bin/ is gitignored — infra, never vendored)
	@mkdir -p $(dir $(RANKE_FETCHER))
	@curl -fsSL $(RANKE_FETCHER_URL) -o $(RANKE_FETCHER)
	@chmod +x $(RANKE_FETCHER)

$(RELEASE_CYCLER): ## Cache release-cycle.sh from ranke-graph (bin/ is gitignored — infra, never vendored)
	@mkdir -p $(dir $(RELEASE_CYCLER))
	@curl -fsSL $(RELEASE_CYCLER_URL) -o $(RELEASE_CYCLER)
	@chmod +x $(RELEASE_CYCLER)

# Read by the release workflow, so bumping TYPST_VERSION here moves CI's install with it.
print-typst-version:
	@[ -n "$(TYPST_VERSION)" ] || { echo "TYPST_VERSION is empty — CI would install whatever typst is latest" >&2; exit 1; }
	@echo $(TYPST_VERSION)

docs: docs-papers docs-pdf ## Pull the ranke-graph documents, then build this repo's handbook (dist/docs.pdf)

docs-papers: $(RANKE_FETCHER) ## Pull papers, spec, glossary into docs/papers/, and place docs/{vocabulary,handbook}.typ (needs git)
	@RANKE_GRAPH_REPO=$(RANKE_GRAPH_REPO) RANKE_GRAPH_REF=$(RANKE_GRAPH_REF) \
		PAPERS_DIR=$(PAPERS_DIR) DOCS_DIR=$(DOCS_DIR) $(RANKE_FETCHER)

docs-current: $(RANKE_FETCHER) ## Re-place docs/{vocabulary,handbook}.typ, refetching only if ranke-graph moved (one git ls-remote) — what verify depends on
	@RANKE_GRAPH_REPO=$(RANKE_GRAPH_REPO) RANKE_GRAPH_REF=$(RANKE_GRAPH_REF) \
		PAPERS_DIR=$(PAPERS_DIR) DOCS_DIR=$(DOCS_DIR) $(RANKE_FETCHER) --if-moved

# docs-papers/docs-current deliberately track main — verify wants the LATEST spec, so
# a drift shows up the moment it happens rather than at ranke-graph's next release
# (see verify's own note). docs-release is the other case: a build with no git, or one
# that wants documents traced to a released version rather than main's tip. Separate
# targets because the two want opposite things from "which ranke-graph".
docs-release: $(RANKE_FETCHER) ## Pull documents from ranke-graph's RANKE_GRAPH_RELEASE release tarball — no git needed
	@PAPERS_DIR=$(PAPERS_DIR) DOCS_DIR=$(DOCS_DIR) $(RANKE_FETCHER) --release $(RANKE_GRAPH_RELEASE)

docs-check: docs-current ## Hold docs/ to the Ranke Documentation Format — compiling proves nothing about it
	@PAPERS_DIR=$(PAPERS_DIR) DOCS_DIR=$(DOCS_DIR) ./scripts/check-docs.sh

docs-pdf: docs-check ## Build dist/docs.pdf from docs/ through shared/handbook.typ (needs typst; DOCS_VERSION prints in place of "dev")
	@command -v $(TYPST) >/dev/null 2>&1 || { echo "missing: typst → $(TYPST_URL) (pin v$(TYPST_VERSION))"; exit 1; }
	@have=$$($(TYPST) --version | awk 'NR==1 {print $$2}'); have=$${have:-unknown}; \
		[ "$$(echo "$$have" | cut -d. -f1,2)" = "$(TYPST_SERIES)" ] || \
		{ echo "typst $$have, but the handbook is built with the $(TYPST_SERIES) series → $(TYPST_URL)"; exit 1; }
	@mkdir -p $(DIST_DIR)
	@$(TYPST) compile --root . --input version=$(DOCS_VERSION) $(DOCS_DIR)/index.typ $(DOCS_PDF)
	@echo ">> wrote $(DOCS_PDF)"

# What this repo AUTHORED and nothing else: the chapters, and the REST contract
# generated from this repo's own openapi.yaml. Anything fetched from ranke-graph
# — the templates, the papers, rql.schema.json — is published by ranke-graph,
# and a second copy here would be a second source of truth.
#
# The generated spec is committed and held by check-generated, so a plain checkout
# has the contract this ships without running the generation toolchain.
docs-bundle: docs-check ## Pack this repo's own chapters and REST contract into dist/ (nothing fetched or supplied)
	@rm -rf $(DIST_DIR)/$(DOCS_BUNDLE_NAME)
	@mkdir -p $(DIST_DIR)/$(DOCS_BUNDLE_NAME)/openapi
	@cp $(DOCS_DIR)/index.typ $(DOCS_DIR)/[0-9]*.typ $(DIST_DIR)/$(DOCS_BUNDLE_NAME)/
	@if [ -d $(DOCS_DIR)/assets ]; then cp -R $(DOCS_DIR)/assets $(DIST_DIR)/$(DOCS_BUNDLE_NAME)/; fi
	@cp $(OPENAPI_GEN) $(DIST_DIR)/$(DOCS_BUNDLE_NAME)/openapi/
	@tar -C $(DIST_DIR) -czf $(DOCS_BUNDLE) $(DOCS_BUNDLE_NAME)
	@rm -rf $(DIST_DIR)/$(DOCS_BUNDLE_NAME)
	@echo ">> wrote $(DOCS_BUNDLE) — $$(tar -tzf $(DOCS_BUNDLE) | grep -cv '/$$') file(s)"

docs-clean: ## Remove the pulled paper references, the built handbook and the packed chapters
	rm -rf $(PAPERS_DIR) $(DOCS_DIR)/vocabulary.typ $(DOCS_DIR)/handbook.typ $(DOCS_PDF) $(DOCS_BUNDLE)
