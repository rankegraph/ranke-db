# ranke-explorer — the browser client.
#
# `make` builds the distributable artifacts; `make dev` starts the dev server.
# Dependencies install themselves on first use, so a clean checkout needs nothing
# but node.
#
# The pipeline: src/ --[vite build]--> .build/ --[inline.mjs]--> dist/explorer.html.
# .build/ is vite's own scratch output (gitignored, rebuilt by `make build`) — never
# committed, never served, never referenced from outside this Makefile. dist/ holds the
# one artifact that matters past this directory: explorer.html, single-file and
# self-contained, committed (like the repo's other generated artifacts) precisely
# because a built artifact never shares a directory with source. `ranke-db` embeds it
# into its release binary (-> ../adapters/endpoints/rest_http).

.PHONY: all dev build single bench bench-render check lint test clean help upgrade ranke-ts-version
.DEFAULT_GOAL := all

VITE  := node node_modules/vite/bin/vite.js
NODE  := node --expose-gc --max-old-space-size=4096

# brokkr, installed on demand rather than assumed present. frontend/ is part of this one
# repo, not a separate project, so there is one cache for the whole thing — the root
# Makefile's bin/tools/ (gitignored there) — rather than a second copy of the same tool.
BROKKR            := ../bin/tools/brokkr
BROKKR_INSTALL_SH := https://raw.githubusercontent.com/flocko-motion/sindri/master/scripts/install-brokkr.sh

all: build single ## Default: .build/ (scratch) and dist/explorer.html (the distributable)

help: ## Show this help
	@grep -hE '^[a-zA-Z_-]+:.*?## ' $(MAKEFILE_LIST) \
		| awk 'BEGIN{FS=":.*?## "}{printf "  \033[36m%-13s\033[0m %s\n", $$1, $$2}'

# Dependencies. Stamped by the directory itself, so it reinstalls when the
# manifests change and stays quiet otherwise.
node_modules: package.json package-lock.json
	@echo ">> installing dependencies"
	@npm install --no-audit --no-fund
	@touch node_modules

# The version shown under the wordmark, stamped the way the server stamps its binary.
# A bare `vite build` gets none and says "dev" rather than claiming a release.
EXPLORER_VERSION ?= $(shell git describe --tags --always --dirty 2>/dev/null || echo dev)
export EXPLORER_VERSION

dev: node_modules ## Start the dev server (open the URL it prints, then press 'load')
	@echo ">> sources live in the Server tab: a mock archive, or a ranke-db instance"
	@$(VITE) --host

build: node_modules ## Bundle to .build/ — vite's own scratch output, input to `single`
	@$(VITE) build

single: build ## Fold .build/ into dist/explorer.html, one self-contained file
	@node scripts/inline.mjs

bench: node_modules ## Headless performance numbers (build, heap, layout, shape)
	@$(NODE) src/bench/graph-bench.ts $(ARGS)

bench-render: node_modules ## Frame timings in a real browser (needs a GPU; skips cleanly)
	@node src/bench/browser-bench.ts $(ARGS)

check: node_modules ## Type-check without emitting, then lint
	@node_modules/typescript/bin/tsc --noEmit
	@$(MAKE) --no-print-directory lint

# deadcode excluded: it needs a main package to trace reachability from, and this
# directory (JS-first, with two incidental Go embed shims for ranke-db's explorer
# route) has none in its own scope. The root `make check` already covers it correctly
# — real main packages in scope there, frontend.Explorer reachable through the real
# call chain — so running it again scoped to just here adds nothing and only breaks.
lint: ## Run brokkr lint over this directory — one already on PATH, else this repo's shared cached copy at ../bin/tools/ (the installer checks GitHub for a newer release every run, skipping the download itself when already current)
	@if command -v brokkr >/dev/null 2>&1; then bin=brokkr; \
	else \
		command -v curl >/dev/null 2>&1 || { echo "ERROR: brokkr not found and curl is not on PATH to install it"; exit 1; }; \
		curl -fsSL $(BROKKR_INSTALL_SH) | bash -s -- $(BROKKR); \
		bin=$(BROKKR); \
	fi; \
	for l in loc comments comment-length gofmt js openspec; do "$$bin" lint "$$l" || exit 1; done

# Node's own runner, which strips the types — the same way the benches run. Core is
# headless, so its behaviour is testable without a DOM or a browser.
#
# Types are transformed rather than only stripped: the generated client declares an enum,
# which strip-only mode refuses outright, and core/data reaches the contract through it. The
# flag is experimental and warns on every run — the choice is deliberate, so the warning goes.
test: node_modules ## Run the headless core tests
	@node --experimental-transform-types --disable-warning=ExperimentalWarning --test 'src/**/*.test.ts'

clean: ## Remove build output (explorer.html is committed, so it stays)
	@rm -rf .build

# --- Dependencies ----------------------------------------------------------

# ranke-ts is the model this client is built on — the same claims, ids and taxonomy the
# server holds through ranke-go — so it is pinned exactly and bumped deliberately, the way
# the module is next door (see ../scripts/upgrade.sh).
#
# It is fetched LAST: `npm update` has already moved everything within its range, so the
# final install is what makes an explicit pin stick.
#
#   make upgrade                       # everything, ranke-ts to latest
#   RANKE_TS_VERSION=0.5.0 make upgrade  # take that version instead
#
# Revert with: git checkout package.json package-lock.json
RANKE_TS         := @rankegraph/ranke
RANKE_TS_VERSION ?= latest

upgrade: node_modules ## Upgrade every dependency and ranke-ts, then check, test and build (RANKE_TS_VERSION=x.y.z)
	@echo ">> dependencies: npm update"
	@npm update --save --no-audit --no-fund
	@echo ">> $(RANKE_TS): fetching $(RANKE_TS_VERSION)"
	@npm install --save-exact --no-audit --no-fund $(RANKE_TS)@$(RANKE_TS_VERSION)
	@echo ">> package.json changes:"
	@git diff -U0 -- package.json | grep -E '^[-+][^-+]' | sed 's/^/     /' || echo "     (none)"
	@echo ">> verifying the upgrade"
	@$(MAKE) --no-print-directory check
	@$(MAKE) --no-print-directory test
	@$(MAKE) --no-print-directory build
	@echo ">> upgrade OK — review the diff, then commit package.json/package-lock.json"

ranke-ts-version: node_modules ## Report whether a newer ranke-ts release exists
	@cur=$$(node -p "require('./package.json').dependencies['$(RANKE_TS)']"); \
	latest=$$(npm view $(RANKE_TS) version 2>/dev/null); \
	if [ -n "$$latest" ] && [ "$$cur" != "$$latest" ]; then \
		echo ">> ranke-ts: on $$cur, latest is $$latest — bump: make upgrade"; \
	else \
		echo ">> ranke-ts: on $$cur, which is the latest"; \
	fi
