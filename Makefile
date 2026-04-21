PREFIX=/usr/local
SUDO=sudo

.PHONY: test
test:
	go test ./...

.PHONY: build
build:
	mkdir -p build
	go build -buildvcs=false -ldflags "-s" -o build/xpass ./cmd/xpass

.PHONY: run
run:
	go run ./...

.PHONY: race
race:
	go run -race ./...

.PHONY: clean
clean:
	rm -rf build

.PHONY: lint
lint:
	go tool golangci-lint run ./...

.PHONY: tidy
tidy:
	go mod tidy

.PHONY: install
install: build
	$(SUDO) install ./build/xpass $(PREFIX)/bin
