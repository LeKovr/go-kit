# Makefile.mk included in golang package Makefiles

GO          ?= go

.PHONY: all help lint vet test cov-html clean

all: help

# ------------------------------------------------------------------------------
## Compile
#:


## run `golint` and `golangci-lint`
lint:
	@golint ./...
	@golangci-lint run ./...

## run `go vet`
vet:
	$(GO) vet ./...

## run tests
test: clean coverage.out

coverage.out: $(SOURCES)
	@echo "*** $@ ***" ; \
	$(GO) test -tags $(TEST_TAGS)$(TEST_TAGS_MORE) -covermode=atomic -coverprofile=$@ ./...

## show package coverage in html
cov-html: coverage.out
	$(GO) tool cover -html=coverage.out

## clean generated files
clean:
	@echo "*** $@ ***" ; \
	[ ! -f coverage.out ] || rm coverage.out

# ------------------------------------------------------------------------------
## Other
#:

# This code handles group header and target comment with one or two lines only
## list Makefile targets
## (this is default target)
help:
	@grep -A 1 -h "^## " $(MAKEFILE_LIST) \
  | sed -E 's/^--$$// ; /./{H;$$!d} ; x ; s/^\n## ([^\n]+)\n(## (.+)\n)*(.+):(.*)$$/"    " "\4" "\1" "\3"/' \
  | sed -E 's/^"    " "#" "(.+)" "(.*)"$$/"" "" "" ""\n"\1 \2" "" "" ""/' \
  | xargs printf "%s\033[36m%-15s\033[0m %s %s\n"
