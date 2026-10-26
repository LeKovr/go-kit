FROM golang:1.25.12-alpine3.23 AS build

WORKDIR /src
COPY config/*.go config/go.mod config/go.sum ./config/
COPY server/*.go server/go.mod server/go.sum ./server/
COPY slogger/*.go slogger/go.mod slogger/go.sum ./slogger/
COPY observability/*.go observability/go.mod observability/go.sum ./observability/
COPY observability/example/*.go observability/example/go.mod observability/example/go.sum ./observability/example/
COPY observability/example/api/*.go ./observability/example/api/

WORKDIR /src/observability/example
RUN --mount=type=cache,target=/go/pkg/mod \
    --mount=type=cache,target=/root/.cache/go-build \
    CGO_ENABLED=0 GOWORK=off go build -mod=readonly -trimpath -o /out/example .

FROM alpine:3.22
WORKDIR /app
COPY --from=build /out/example /usr/local/bin/example
ENTRYPOINT ["example"]
