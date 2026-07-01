.PHONY: proto-gen test lint

proto-gen:
	@command -v protoc >/dev/null || (echo "protoc required"; exit 1)
	@mkdir -p internal/grpc/pb
	PATH="$(shell go env GOBIN):$(shell go env GOPATH)/bin:$(CURDIR)/bin:$$PATH" protoc -I ./proto \
		--go_out=./internal/grpc/pb --go_opt=paths=source_relative \
		--go-grpc_out=./internal/grpc/pb --go-grpc_opt=paths=source_relative \
		proto/imgvalidation/v1/img_validation.proto

test:
	go test -race -coverprofile=coverage.out ./...

lint:
	golangci-lint run --timeout=5m
