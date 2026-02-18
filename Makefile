.PHONY: run build templ-gen clean prod-build build-mcp

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
