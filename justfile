# prints all available recipes
default:
    @just --list

# runs the entry point using "go run"
run *args:
    go run cmd/main.go {{ args }}

# builds the executable in build/minienv
build:
    CGO_ENABLED=0 go build \
      -ldflags="-s -w" \
      -trimpath \
      -o build/minienv \
      cmd/main.go
