# Development tasks. CI (.github/workflows/ci.yml) and the pre-commit hook
# (.githooks/pre-commit) run these same targets, so local, hook, and CI
# behavior always match.
#
# One-time setup:   make hooks
# Everything CI does: make check

GO ?= go

# Analysis tools are run via `go run pkg@latest`: no install step, and the
# module cache makes reruns fast. Pin a version here if @latest ever breaks.
STATICCHECK := honnef.co/go/tools/cmd/staticcheck@latest
ERRCHECK    := github.com/kisielk/errcheck@latest
GOVULNCHECK := golang.org/x/vuln/cmd/govulncheck@latest

.PHONY: build test test-quick check fmt fmt-check vet staticcheck errcheck tidy-check vuln hooks

build:
	$(GO) build ./...

# Full test run with the race detector (what CI runs).
test:
	$(GO) test -race ./...

# Faster test run for the pre-commit hook (no race-instrumented rebuild).
test-quick:
	$(GO) test ./...

fmt:
	gofmt -w .

fmt-check:
	@out="$$(gofmt -l .)"; if [ -n "$$out" ]; then \
		echo "files need gofmt:" >&2; echo "$$out" >&2; exit 1; fi

vet:
	$(GO) vet ./...

staticcheck:
	$(GO) run $(STATICCHECK) ./...

# Unchecked error returns. Deliberate exceptions live in .errcheck-excludes.
errcheck:
	$(GO) run $(ERRCHECK) -exclude .errcheck-excludes ./...

tidy-check:
	$(GO) mod tidy -diff

# Call-graph-aware vulnerability scan: fails only when our code actually
# reaches a vulnerable function, not on any dependency advisory.
vuln:
	$(GO) run $(GOVULNCHECK) ./...

# The full battery — identical to CI.
check: build fmt-check vet staticcheck errcheck tidy-check vuln test

# Point git at the repo's hooks (pre-commit runs the fast checks).
hooks:
	git config core.hooksPath .githooks
	@echo "installed: git config core.hooksPath .githooks"
