module github.com/LeKovr/go-kit/server/example

go 1.25.0

require (
	github.com/LeKovr/go-kit/config v0.0.0
	github.com/LeKovr/go-kit/server v0.0.0
	github.com/LeKovr/go-kit/server/grpc v0.0.0
	github.com/LeKovr/go-kit/server/http v0.0.0
	google.golang.org/grpc v1.83.1
	google.golang.org/protobuf v1.36.11
)

require (
	github.com/felixge/httpsnoop v1.1.0 // indirect
	github.com/jessevdk/go-flags v1.6.1 // indirect
	golang.org/x/net v0.55.0 // indirect
	golang.org/x/sync v0.21.0 // indirect
	golang.org/x/sys v0.46.0 // indirect
	golang.org/x/text v0.37.0 // indirect
	google.golang.org/genproto/googleapis/rpc v0.0.0-20260526163538-3dc84a4a5aaa // indirect
)

replace github.com/LeKovr/go-kit/server => ..

replace github.com/LeKovr/go-kit/server/http => ../http

replace github.com/LeKovr/go-kit/server/grpc => ../grpc

replace github.com/LeKovr/go-kit/config => ../../config
