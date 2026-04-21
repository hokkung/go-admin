.PHONY: tidy test gen api

tidy:
	@go mod tidy

gen:
	go generate ./...

test:
	@go test -race ./... -cover 2>&1

api:
	@go run example/cmd/main.go
