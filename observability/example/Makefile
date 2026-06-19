SHELL = /bin/sh

GO ?= go
GOWORK ?= off
PRG ?= example
PRG_DEST ?= $(PRG)
CFG ?= .env
CFG_TMPL ?= Makefile.env
ZO_ROOT_USER_EMAIL ?= root@example.com

SOURCES = $(shell find . -maxdepth 1 -name '*.go')
APP_VERSION ?= $(shell git describe --tags --always)
TARGETOS ?= linux
TARGETARCH ?= amd64
LDFLAGS ?= -s -w -extldflags '-static'

.PHONY: config build up down full-down logs clean

## build example binary
build: $(PRG_DEST) $(CFG_TMPL)

$(PRG_DEST): $(SOURCES) go.mod go.sum
	@GOOS=$(TARGETOS) GOARCH=$(TARGETARCH) CGO_ENABLED=$(CGO_ENABLED) GOWORK=$(GOWORK) \
	  $(GO) build -v -o $@ -ldflags \
	  "$(LDFLAGS) -X main.version=$(APP_VERSION)" \
	  .

$(CFG_TMPL): $(PRG_DEST)
	@./$(PRG_DEST) --config_gen=mk > $@ 2>/dev/null

## generate local OpenObserve credentials
config: $(CFG_TMPL)
	@password=Aa1!$$(tr -dc A-Za-z0-9 < /dev/urandom 2>/dev/null | head -c28); \
	auth_digest=$$(printf "%s:%s" "$(ZO_ROOT_USER_EMAIL)" "$$password" | base64 | tr -d '\n'); \
	sed -E 's/[[:space:]]*\?=[[:space:]]*/=/' < $(CFG_TMPL) > $(CFG); \
	{ \
	  echo; \
	  echo "# OpenObserve local credentials"; \
	  echo "ZO_ROOT_USER_EMAIL=$(ZO_ROOT_USER_EMAIL)"; \
	  echo "ZO_ROOT_USER_PASSWORD=$$password"; \
	  echo "OPENOBSERVE_AUTH_DIGEST=$$auth_digest"; \
	} >> $(CFG); \
	echo "Generated $(CFG)"; \
	echo "OpenObserve login: $(ZO_ROOT_USER_EMAIL)"; \
	echo "OpenObserve password: $$password"

## start local telemetry stack
up:
	@if [ -f $(CFG) ]; then \
	  sed -n \
	    -e 's/^ZO_ROOT_USER_EMAIL=/OpenObserve login: /p' \
	    -e 's/^ZO_ROOT_USER_PASSWORD=/OpenObserve password: /p' \
	    $(CFG); \
	else \
	  make config; \
	fi
	docker compose up -d

## stop local telemetry stack
down:
	docker compose down

## stop local telemetry stack and remove volumes
full-down:
	docker compose down -v

## follow collector logs
logs:
	docker compose logs -f otel-collector

## remove generated local config
clean:
	rm -f $(CFG)
