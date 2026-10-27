SHELL = /bin/sh

GO ?= go
GOWORK ?= off
PRG ?= example
PRG_DEST ?= $(PRG)
CFG ?= .env
CFG_TMPL ?= Makefile.env
LOG_DEST ?= error.log
ZO_ROOT_USER_EMAIL ?= root@example.com

SOURCES = $(shell find . -maxdepth 1 -name '*.go') $(wildcard api/*.go)
APP_VERSION ?= $(shell git describe --tags --always)
TARGETOS ?= $(shell $(GO) env GOOS)
TARGETARCH ?= $(shell $(GO) env GOARCH)
LDFLAGS ?= -s -w -extldflags '-static'

-include $(CFG_TMPL)

.PHONY: config build run generate prepare-log-file up requests down full-down logs clean

## build example binary
build: $(PRG_DEST) $(CFG_TMPL)

$(PRG_DEST): $(SOURCES) go.mod go.sum
	@GOOS=$(TARGETOS) GOARCH=$(TARGETARCH) CGO_ENABLED=$(CGO_ENABLED) GOWORK=$(GOWORK) \
	  $(GO) build -v -o $@ -ldflags \
	  "$(LDFLAGS) -X main.version=$(APP_VERSION)" \
	  .

## generate missing config defaults; refresh explicitly with make -B Makefile.env
$(CFG_TMPL):
	@GOWORK=$(GOWORK) $(GO) run . --config_gen=mk > $@ 2>/dev/null

## regenerate protobuf sources; requires protoc and Go plugins in GOPATH/bin or PATH
generate:
	@PATH="$$($(GO) env GOPATH)/bin:$$PATH" protoc \
	  --go_out=. --go_opt=paths=source_relative \
	  --go-grpc_out=. --go-grpc_opt=paths=source_relative api/calculate.proto

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

## build and start the complete example; wait for OpenObserve
up: prepare-log-file
	@if [ -f $(CFG) ]; then \
	  sed -n \
	    -e 's/^ZO_ROOT_USER_EMAIL=/OpenObserve login: /p' \
	    -e 's/^ZO_ROOT_USER_PASSWORD=/OpenObserve password: /p' \
	    $(CFG); \
	else \
	  make config; \
	fi
	LOG_DEST=$(LOG_DEST) docker compose up --build --wait --wait-timeout 120
	@LOG_DEST=$(LOG_DEST) docker compose run --rm --no-deps --entrypoint sh client -ec '\
	  for attempt in $$(seq 1 30); do \
	    if wget -q -T 3 -O /dev/null http://openobserve:5080/healthz; then exit 0; fi; \
	    sleep 1; \
	  done; \
	  echo "OpenObserve is not ready" >&2; exit 1'

## send the demonstration request with HTTP client spans
requests:
	@LOG_DEST=$(LOG_DEST) docker compose run --rm --no-deps client

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
