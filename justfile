# prints all available recipes
default:
    @just --list

# builds the executable in build/minienv
build:
    CGO_ENABLED=0 go build \
      -ldflags="-s -w" \
      -trimpath \
      -o build/minienv \
      cmd/cli/main.go

# lints entire project
lint:
    golangci-lint run

# runs the entry point using "go run"
run *args:
    go run cmd/cli/main.go {{ args }}

# generates schema files
gen-schema:
    go run cmd/schema/main.go
