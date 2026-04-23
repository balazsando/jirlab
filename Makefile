BINARY := jirlab
GO     := go

.PHONY: build fmt lint test clean build-powershell install-powershell

build:
	$(GO) build -o $(BINARY) .

build-powershell:
	GOOS=windows GOARCH=amd64 $(GO) build -tags powershell -o $(BINARY).exe .

fmt:
	$(GO) fmt ./...
	$(GO) run golang.org/x/tools/cmd/goimports@latest -w .

lint:
	golangci-lint run ./...

test:
	$(GO) test ./...

test-coverage:
	$(GO) test -coverprofile=coverage.out ./...
	$(GO) tool cover -html=coverage.out -o coverage.html

clean:
	rm -f $(BINARY) $(BINARY).exe coverage.out coverage.html

install: build
	install -m 0755 $(BINARY) $(HOME)/.local/bin/$(BINARY)

install-powershell: build-powershell
	@echo "Copy $(BINARY).exe to a directory in your PATH (e.g. C:\\Users\\<you>\\bin\\)"
