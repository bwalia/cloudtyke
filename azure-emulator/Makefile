# CloudTyke Azure emulator
SHELL := /bin/bash
VERSION ?= $(shell git describe --tags --always --dirty 2>/dev/null || echo dev)
COMPOSE := docker compose
TOOLBOX := $(COMPOSE) run --rm -T toolbox
EXAMPLES ?= quickstart entra
REPORTS ?= reports
GO_TEST_FLAGS ?= -count=1

.PHONY: help build test test-race test-services test-all selftest lint coverage coverage-check up down clean logs reset env shell tf-e2e tf-e2e-terraform

help: ## show targets
	@grep -E '^[a-zA-Z_-]+:.*?## ' $(MAKEFILE_LIST) | awk 'BEGIN {FS = ":.*?## "}; {printf "  %-18s %s\n", $$1, $$2}'

build: ## build emulator + tyke binaries into ./bin
	CGO_ENABLED=0 go build -trimpath -ldflags "-X github.com/bwalia/cloudtyke/azure-emulator/internal/version.Version=$(VERSION)" -o bin/emulator ./cmd/emulator
	CGO_ENABLED=0 go build -trimpath -o bin/tyke ./cmd/tyke

test: ## unit + integration tests
	go test ./...

test-race: ## tests with the race detector
	go test -race -count=1 ./...

test-services: ## one conformance check per coverage-matrix service (fast, in-process)
	go test -count=1 -v ./test/conformance

test-all: ## every test + HTML/JUnit report in reports/ (SKIP_E2E=1 skips the slow Terraform runs)
	@mkdir -p $(REPORTS) && rm -f $(REPORTS)/e2e-*.log
	@echo "==> unit, integration and per-service tests"
	@go test -json $(GO_TEST_FLAGS) ./... > $(REPORTS)/go-test.json || true
	@if [ -z "$(SKIP_E2E)" ]; then \
	  echo "==> starting emulator"; \
	  if $(MAKE) --no-print-directory up > $(REPORTS)/up.log 2>&1; then \
	    echo "==> Terraform e2e with OpenTofu (a few minutes)"; \
	    $(MAKE) --no-print-directory tf-e2e > $(REPORTS)/e2e-tofu.log 2>&1 || true; \
	    echo "==> Terraform e2e with Terraform (a few minutes)"; \
	    $(MAKE) --no-print-directory tf-e2e-terraform > $(REPORTS)/e2e-terraform.log 2>&1 || true; \
	  else \
	    echo "emulator failed to start, see $(REPORTS)/up.log" | tee $(REPORTS)/e2e-tofu.log > $(REPORTS)/e2e-terraform.log; \
	  fi; \
	fi
	@go run ./test/report -in $(REPORTS)/go-test.json -out $(REPORTS) \
	  -e2e OpenTofu=$(REPORTS)/e2e-tofu.log -e2e Terraform=$(REPORTS)/e2e-terraform.log

selftest: build ## run the live self-test inside the running emulator (also at https://localhost/_cloudtyke/selftest)
	bin/tyke azure selftest

lint: ## gofmt + go vet
	@test -z "$$(gofmt -l .)" || (gofmt -l . && echo "run gofmt -w ." && exit 1)
	go vet ./...

coverage: ## regenerate docs/coverage.md from the registered resource providers
	go run ./cmd/emulator coverage > docs/coverage.md

coverage-check: ## fail if docs/coverage.md is stale
	@go run ./cmd/emulator coverage | diff -u docs/coverage.md - || (echo "docs/coverage.md is stale: run make coverage" && exit 1)

up: ## start emulator + DNS sidecar and export certs/ca.pem
	$(COMPOSE) up -d --build --wait
	@mkdir -p certs && $(COMPOSE) cp emulator:/data/certs/ca.pem certs/ca.pem && echo "CA exported to certs/ca.pem"

down: ## stop containers (state volume is kept)
	$(COMPOSE) --profile tools down

clean: ## stop containers and delete the state volume
	$(COMPOSE) --profile tools down -v

logs: ## follow emulator logs
	$(COMPOSE) logs -f emulator

reset: ## wipe state and re-seed
	curl -fsS --cacert certs/ca.pem -X POST https://localhost/_cloudtyke/reset

env: ## print ARM_* settings for the bootstrap service principal
	@curl -fsS --cacert certs/ca.pem https://localhost/_cloudtyke/env

shell: ## toolbox shell (tofu, terraform) wired to the emulator
	$(COMPOSE) --profile tools run --rm toolbox

tf-e2e: ## run the examples through init/validate/apply/plan(no drift)/destroy with OpenTofu
	$(COMPOSE) --profile tools build toolbox
	$(TOOLBOX) ./test/e2e-terraform/run.sh tofu $(EXAMPLES)

tf-e2e-terraform: ## same with HashiCorp Terraform
	$(COMPOSE) --profile tools build toolbox
	$(TOOLBOX) ./test/e2e-terraform/run.sh terraform $(EXAMPLES)
