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

# Version stamped into release artifacts (see dist). Defaults to git's
# description of HEAD; the release workflow passes the tag explicitly.
VERSION ?= $(shell git describe --tags --always --dirty)

# Platforms release artifacts are built for.
DIST_PLATFORMS := darwin/arm64 darwin/amd64 linux/arm64 linux/amd64

.PHONY: build test test-quick check fmt fmt-check vet staticcheck errcheck tidy-check vuln dist hooks

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

# Cross-compiled release artifacts: dist/wt_<version>_<os>_<arch>.tar.gz plus
# a checksums.txt. The version is injected via ldflags because GOOS/GOARCH
# cross-builds don't get the toolchain's VCS stamping. CGO is disabled for
# static, portable binaries.
dist:
	rm -rf dist && mkdir -p dist
	@for p in $(DIST_PLATFORMS); do \
		os=$${p%/*}; arch=$${p#*/}; \
		out="dist/wt_$(VERSION)_$${os}_$${arch}"; \
		echo "building $$out.tar.gz"; \
		GOOS=$$os GOARCH=$$arch CGO_ENABLED=0 $(GO) build -trimpath \
			-ldflags "-s -w -X main.version=$(VERSION)" \
			-o "$$out/wt" ./cmd/wt || exit 1; \
		tar -C "$$out" -czf "$$out.tar.gz" wt; \
		rm -r "$$out"; \
	done
	cd dist && shasum -a 256 *.tar.gz > checksums.txt

# Point git at the repo's hooks (pre-commit runs the fast checks).
hooks:
	git config core.hooksPath .githooks
	@echo "installed: git config core.hooksPath .githooks"
