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

# runs all test suites
test *args:
    # --fail-on-empty
    ginkgo \
      -r \
      -p \
      --v \
      --randomize-all \
      --randomize-suites \
      --fail-on-pending \
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

# runs the entry point using "go run"
run *args:
    go run cmd/cli/main.go {{ args }}

# generates schema files
gen-schema:
    go run cmd/schema/main.go
