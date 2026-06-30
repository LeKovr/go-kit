SHELL = /bin/sh

GO ?= go
GOWORK ?= off
PRG ?= example
PRG_DEST ?= $(PRG)
CFG ?= .env
CFG_TMPL ?= Makefile.env
LOG_DEST ?= error.log
ZO_ROOT_USER_EMAIL ?= root@example.com

SOURCES = $(shell find . -maxdepth 1 -name '*.go')
APP_VERSION ?= $(shell git describe --tags --always)
TARGETOS ?= $(shell $(GO) env GOOS)
TARGETARCH ?= $(shell $(GO) env GOARCH)
LDFLAGS ?= -s -w -extldflags '-static'

-include $(CFG_TMPL)

.PHONY: config build run prepare-log-file up down full-down logs clean

## build example binary
build: $(PRG_DEST)

$(PRG_DEST): $(SOURCES) go.mod go.sum
	@GOOS=$(TARGETOS) GOARCH=$(TARGETARCH) CGO_ENABLED=$(CGO_ENABLED) GOWORK=$(GOWORK) \
	  $(GO) build -v -o $@ -ldflags \
	  "$(LDFLAGS) -X main.version=$(APP_VERSION)" \
	  .

## regenerate config defaults template
$(CFG_TMPL): $(SOURCES) go.mod go.sum Makefile
	@GOWORK=$(GOWORK) $(GO) run . --config_gen=mk > $@ 2>/dev/null

## run example app with traces, metrics, runtime metrics and JSON logs
run: prepare-log-file
	@$(GO) run . \
	  --mode=$(MODE) \
	  --otel.enable_traces \
	  --otel.enable_metrics \
	  --otel.enable_go_runtime_metrics \
	  --log.format=json \
	  --log.debug \
	  --log.dest=$(LOG_DEST) \
	  --http.access_log=-

## create log file for collector filelog receiver
prepare-log-file:
	@if [ -n "$(LOG_DEST)" ]; then \
	  mkdir -p "$$(dirname "$(LOG_DEST)")"; \
	  touch "$(LOG_DEST)"; \
	  test -f "$(LOG_DEST)" || { echo "Log file was not created: $(LOG_DEST)" >&2; exit 1; }; \
	fi

## generate local OpenObserve credentials
config: $(CFG_TMPL) prepare-log-file
	@# LC_ALL=C makes tr process /dev/urandom bytes safely on macOS UTF-8 locales.
	@password=Aa1!$$(LC_ALL=C tr -dc A-Za-z0-9 < /dev/urandom 2>/dev/null | head -c28); \
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
up: prepare-log-file
	@if [ -f $(CFG) ]; then \
	  sed -n \
	    -e 's/^ZO_ROOT_USER_EMAIL=/OpenObserve login: /p' \
	    -e 's/^ZO_ROOT_USER_PASSWORD=/OpenObserve password: /p' \
	    $(CFG); \
	else \
	  make config; \
	fi
	LOG_DEST=$(LOG_DEST) docker compose up -d

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
