GO ?= go
GOFMT ?= gofmt
GO_PACKAGES := ./...

# CI tools are invoked through immutable Go module versions. govulncheck is
# maintained by the Go security team (BSD-3-Clause), go-licenses by Google
# (Apache-2.0), and actionlint and Gitleaks by their established upstream
# projects (MIT).
GOVULNCHECK := $(GO) run golang.org/x/vuln/cmd/govulncheck@v1.6.0
GO_LICENSES := $(GO) run github.com/google/go-licenses/v2@v2.0.1
ACTIONLINT := $(GO) run github.com/rhysd/actionlint/cmd/actionlint@v1.7.12
GITLEAKS := $(GO) run github.com/zricethezav/gitleaks/v8@v8.28.0
GORELEASER := $(GO) run github.com/goreleaser/goreleaser/v2@v2.13.3

TEST_TIMEOUT := 180s
TEST_PARALLEL := 10
TESTACC_TIMEOUT := 120m

default: build

fmt:
	$(GOFMT) -s -w -e .

fmt-check:
	@test -z "$$($(GOFMT) -l .)" || { \
		$(GOFMT) -d .; \
		echo "Go files are not formatted; run 'make fmt'." >&2; \
		exit 1; \
	}

lint:
	$(GO) vet $(GO_PACKAGES)

test:
	$(GO) test -count=1 -v -cover -timeout=$(TEST_TIMEOUT) -parallel=$(TEST_PARALLEL) $(GO_PACKAGES)

test-race:
	$(GO) test -race -count=1 -timeout=$(TEST_TIMEOUT) -parallel=$(TEST_PARALLEL) $(GO_PACKAGES)

testacc: export TF_ACC := 1
testacc:
	$(GO) test -count=1 -v -cover -timeout=$(TESTACC_TIMEOUT) $(GO_PACKAGES)

build:
	$(GO) build -trimpath -v $(GO_PACKAGES)

modules-check:
	$(GO) mod tidy -diff
	$(GO) mod verify
	$(GO) list -mod=readonly -m all > /dev/null

docs:
	$(GO) run ./internal/tools/docsgen -write

examples-check:
	$(GO) run ./internal/tools/examplecheck

docs-check:
	$(GO) run ./internal/tools/docsgen
	$(GO) run ./internal/tools/examplecheck

workflow-check:
	$(ACTIONLINT)

release-config-check:
	$(GORELEASER) check

snapshot:
	$(GORELEASER) release --snapshot --clean --skip=sign

licenses-check:
	$(GO_LICENSES) check --include_tests $(GO_PACKAGES)

vulnerability-check:
	$(GOVULNCHECK) -test $(GO_PACKAGES)

secrets-check:
	$(GITLEAKS) detect --source . --no-banner --redact --no-git

supply-chain-check: modules-check licenses-check vulnerability-check secrets-check

ci: fmt-check lint test test-race build modules-check docs-check workflow-check licenses-check vulnerability-check secrets-check

.PHONY: default fmt fmt-check lint test test-race testacc build modules-check \
	docs examples-check docs-check workflow-check release-config-check snapshot \
	licenses-check vulnerability-check secrets-check supply-chain-check ci
