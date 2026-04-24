.PHONY: tidy test gen api api-gen admin-gen admin-gen-install

tidy:
	@go mod tidy

gen:
	go generate ./...

test:
	@go test -race ./... -cover 2>&1

api:
	@go run ./example/cmd/admin/

api-gen:
	@go run ./example/cmd/admin-gen/

# Regenerate the admin-gen example admin/ directory from its annotated models.
# Point IN/OUT at your project's dirs when using outside this repo.
IN  ?= ./example/cmd/admin-gen/models
OUT ?= ./example/cmd/admin-gen/admin
admin-gen:
	@go run ./admin-gen/cmd/admin-gen/ -in $(IN) -out $(OUT)

# One-off: install the CLI onto your PATH via `go install`.
admin-gen-install:
	@go install ./admin-gen/cmd/admin-gen/
	@echo "installed to $$(go env GOPATH)/bin/admin-gen"
