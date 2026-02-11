SHELL          = /bin/sh
CFG           := .env
PRG           := $(shell basename $$PWD)
PRG_DEST      := $(PRG)
CFG_TMPL      := Makefile.env

# -----------------------------------------------------------------------------
# Build config

ifdef NOGIT

else
SOURCES        := $(shell find . -maxdepth 4 -mindepth 1 -path ./data -prune -o -name '*.go')

APP_VERSION   := $(shell git describe --tags --always)
# Last project tag (used in `make chlog`)
RELEASE       := $(shell git describe --tags --abbrev=0 --always)
# Repository address (compiled into main.repo)
REPO          := $(shell git config --get remote.origin.url)
endif

GO             := go
GOLANG_IMAGE   := registry.gitflic.ru/company/dcape/golang-alpine
GOLANG_VERSION := v1.24.4-alpine3.22.1
GOLANGCI_IMAGE := registry.gitflic.ru/project/dcape/gocilint/gocilint

TARGETOS      := linux
TARGETARCH    := amd64
LDFLAGS       := -s -w -extldflags '-static'

# Distros: arch
ALLARCH       := "linux/amd64 linux/386 darwin/amd64 linux/arm linux/arm64"
# Distros: destination
DIRDIST       := dist

# Path to golang package docs
GODOC_REPO    := github.com/!le!kovr/$(PRG)
# App docker image
DOCKER_IMAGE  := ghcr.io/lekovr/$(PRG)

.PHONY: build build-standalone run chlog

# ------------------------------------------------------------------------------
## Compile operations
#:

## Build app
build: $(PRG_DEST) config.md

# Use: make build PRG_DEST=app-static
%-static: CGO_ENABLED=0

$(PRG_DEST): $(SOURCES)
	@GOOS=$${TARGETOS} GOARCH=$${TARGETARCH} CGO_ENABLED=$(CGO_ENABLED) \
	  $(GO) build -v -o $@ -ldflags \
	 "$${LDFLAGS} -X main.version=$(APP_VERSION) -X main.repo=$(REPO)" \
	 . && \
	 ./$(PRG_DEST) --config_gen=mk > $(CFG_TMPL) 2>/dev/null

## Changes from last tag
chlog:
	@echo Changes since $(RELEASE)
	@echo
	@git log $(RELEASE)..@ --pretty=format:"* %s"

## generate CLI options report
config.md: $(PRG_DEST)
	@./$(PRG_DEST) --config_gen=md > $@ 2>/dev/null || true

