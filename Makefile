.PHONY: run build templ-gen clean prod-build build-mcp test lint docker docker-run

run: templ-gen
	go run ./cmd/ezweb

build: templ-gen
	go build -o ezweb ./cmd/ezweb

build-mcp:
	go build -o ezweb-mcp ./cmd/ezweb-mcp

templ-gen:
	templ generate

clean:
	rm -f ezweb ezweb-mcp ezweb.db

prod-build: templ-gen
	CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -ldflags="-s -w" -o ezweb ./cmd/ezweb

test:
	go test ./... -v -count=1

lint:
	go vet ./...
	@which staticcheck > /dev/null 2>&1 && staticcheck ./... || echo "staticcheck not installed, skipping"

docker: templ-gen
	docker build -t ezweb .

docker-run: docker
	docker run --rm -p 3000:3000 --env-file .env ezweb
