####
##  Dependency versions
####

# renovate: datasource=github-releases depName=golangci/golangci-lint versioning=semver
GOLANGCI_LINT_VERSION := 2.12.2

# renovate: datasource=go depName=github.com/goph/licensei versioning=semver
LICENSEI_VERSION = 0.9.0

BIN := ${PWD}/bin

export PATH := $(BIN):$(PATH)

GOVERSION := $(shell go env GOVERSION)

GOLANGCI_LINT := $(BIN)/golangci-lint
LICENSEI := ${BIN}/licensei

# Image URL to use all building/pushing image targets
CONTAINER_TOOL ?= docker
IMG ?= eventrouter:local

# Get the currently used golang install path (in GOPATH/bin, unless GOBIN is set)
ifeq (,$(shell go env GOBIN))
GOBIN=$(shell go env GOPATH)/bin
else
GOBIN=$(shell go env GOBIN)
endif

# Setting SHELL to bash allows bash commands to be executed by recipes.
# Options are set to exit when a recipe line exits non-zero or a piped command fails.
SHELL = /usr/bin/env bash -o pipefail
.SHELLFLAGS = -ec

##@ General

.DEFAULT_GOAL = help
.PHONY: help
help: ## Display this help.
	@awk 'BEGIN {FS = ":.*##"; printf "\nUsage:\n  make \033[36m<target>\033[0m\n"} /^[a-zA-Z_0-9-]+:.*?##/ { printf "  \033[36m%-15s\033[0m %s\n", $$1, $$2 } /^##@/ { printf "\n\033[1m%s\033[0m\n", substr($$0, 5) } ' $(MAKEFILE_LIST)

##@ Development

.PHONY: fmt
fmt: ## Run go fmt against code.
	go fmt ./...

.PHONY: vet
vet: ## Run go vet against code.
	go vet ./...

.PHONY: test
test: ## Run tests.
	go test -timeout 60s ./...

.PHONY: tidy
tidy: ## Tidy Go modules
	find . -iname "go.mod" -not -path "./.devcontainer/*" | xargs -L1 sh -c 'cd $$(dirname $$0); go mod tidy'

.PHONY: lint
lint: ${GOLANGCI_LINT} ## Run golangci-lint
	${GOLANGCI_LINT} run

.PHONY: lint-fix
lint-fix: ${GOLANGCI_LINT} ## Run golangci-lint and perform fixes
	${GOLANGCI_LINT} run --fix

.PHONY: check-diff
check-diff: tidy fmt ## Verify go.mod tidiness and formatting are committed.
	git diff --exit-code

.PHONY: license-cache
license-cache: ${LICENSEI} ## Generate license cache
	${LICENSEI} cache

.PHONY: license-check
license-check: ${LICENSEI} .licensei.cache ## Run license check
	${LICENSEI} check
	${LICENSEI} header

##@ Build

.PHONY: build
build: fmt vet build-binary ## Build eventrouter binary (with fmt + vet).

.PHONY: build-binary
build-binary: ## Compile the eventrouter binary only (used by the container build).
	go build -o eventrouter ./cmd

.PHONY: run
run: fmt vet ## Run the eventrouter from your host.
	go run ./cmd

.PHONY: docker-build
docker-build: ## Build docker image with the eventrouter.
	$(CONTAINER_TOOL) build -t ${IMG} .

##@ Build Dependencies

${GOLANGCI_LINT}: ${GOLANGCI_LINT}_${GOLANGCI_LINT_VERSION}_${GOVERSION} | ${BIN}
	ln -sf $(notdir $<) $@

${GOLANGCI_LINT}_${GOLANGCI_LINT_VERSION}_${GOVERSION}: IMPORT_PATH := github.com/golangci/golangci-lint/v2/cmd/golangci-lint
${GOLANGCI_LINT}_${GOLANGCI_LINT_VERSION}_${GOVERSION}: VERSION := v${GOLANGCI_LINT_VERSION}
${GOLANGCI_LINT}_${GOLANGCI_LINT_VERSION}_${GOVERSION}: | ${BIN}
	${go_install_binary}

${LICENSEI}: ${LICENSEI}_${LICENSEI_VERSION}_${GOVERSION} | ${BIN}
	ln -sf $(notdir $<) $@

${LICENSEI}_${LICENSEI_VERSION}_${GOVERSION}: IMPORT_PATH := github.com/goph/licensei/cmd/licensei
${LICENSEI}_${LICENSEI_VERSION}_${GOVERSION}: VERSION := v${LICENSEI_VERSION}
${LICENSEI}_${LICENSEI_VERSION}_${GOVERSION}: | ${BIN}
	${go_install_binary}

.licensei.cache: ${LICENSEI}
ifndef GITHUB_TOKEN
	@>&2 echo "WARNING: building licensei cache without Github token, rate limiting might occur."
	@>&2 echo "(Hint: If too many licenses are missing, try specifying a Github token via the environment variable GITHUB_TOKEN.)"
endif
	${LICENSEI} cache

${BIN}:
	mkdir -p bin

define go_install_binary
find ${BIN} -name '$(notdir ${IMPORT_PATH})_*' -exec rm {} +
GOBIN=${BIN} go install ${IMPORT_PATH}@${VERSION}
mv ${BIN}/$(notdir ${IMPORT_PATH}) $@
endef
