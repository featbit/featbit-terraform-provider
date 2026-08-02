GO ?= go
GOFMT ?= gofmt
GO_PACKAGES := ./...

TEST_TIMEOUT := 120s
TEST_PARALLEL := 10
TESTACC_TIMEOUT := 120m

default: build

fmt:
	$(GOFMT) -s -w -e .

lint:
	$(GO) vet $(GO_PACKAGES)

test:
	$(GO) test -count=1 -v -cover -timeout=$(TEST_TIMEOUT) -parallel=$(TEST_PARALLEL) $(GO_PACKAGES)

testacc: export TF_ACC := 1
testacc:
	$(GO) test -count=1 -v -cover -timeout=$(TESTACC_TIMEOUT) $(GO_PACKAGES)

build:
	$(GO) build -v $(GO_PACKAGES)

.PHONY: default fmt lint test testacc build
