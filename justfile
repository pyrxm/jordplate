# List available recipes
default:
  @just --list

# Build the binary
[group('build')]
build:
  go build -o jordplate main.go

[group('test')]
vet:
  go vet ./...

[group('test')]
test:
  go test -race -count=1 ./...
