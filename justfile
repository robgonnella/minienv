# prints all available recipes
default:
    @just --list

# builds the executable in build/minienv
build:
    CGO_ENABLED=0 go build \
      -ldflags="-s -w" \
      -trimpath \
      -o build/minienv \
      cmd/minienv/main.go

# builds and installs executable locally
install:
    go install ./cmd/minienv

# lints entire project
lint:
    golangci-lint run

# formats entire project
fmt:
    golangci-lint fmt ./...

# runs all test suites
test *args:
    cd {{ invocation_directory() }} \
    && ginkgo \
      -r \
      -p \
      --v \
      --randomize-all \
      --randomize-suites \
      --fail-on-pending \
      --fail-on-empty \
      --keep-going \
      --cover \
      --coverprofile=cover.profile \
      --race \
      --trace \
      --json-report=report.json \
      --output-dir=reports {{ args }}

# generates and opens html coverage report
coverage-report:
    go tool cover \
      -html=reports/cover.profile \
      -o reports/coverage.html \
      && open reports/coverage.html

# generates mocks - re-run this anytime a mocked interface is updated
mock:
    mockery

# generates schema files
gen-schema:
    go run cmd/schema/main.go

# deploys the minienv defined in compose.yml
up *args:
    @just _run up {{ args }}

# destroys the minienv defined in compose.yml
down *args:
    @just _run down {{ args }}

# builds the documentation book
docs:
    mdbook build docs

# serves the documentation book locally with live reload
docs-serve:
    mdbook serve docs --open

# runs the entry point using "go run"
_run *args:
    go run cmd/minienv/main.go {{ args }}
