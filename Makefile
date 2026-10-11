.PHONY: fmt tidy build test lint check web web-fresh

# The rooms interface: web/_app builds into web/dist, which the binary embeds.
# The build is committed; web-fresh fails when a rebuild differs from it.
web:
	cd web/_app && npm ci --no-audit --no-fund && npm run check

web-fresh: web
	git diff --exit-code -- web/dist || (echo "web/dist is stale: commit the rebuilt interface" && exit 1)

# Format all Go source (gofmt); golangci-lint's formatters also cover goimports.
fmt:
	gofmt -w .

# Keep go.mod/go.sum honest.
tidy:
	go mod tidy

build:
	go build ./...

# All tests, no skips, race detector on.
test:
	go test -race ./...

lint:
	golangci-lint run

# The one gate to run before every commit: everything green.
check: web-fresh fmt tidy build test lint
