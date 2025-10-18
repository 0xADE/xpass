.PHONY: build
build:
	mkdir -p build
	go build -buildvcs=false -ldflags "-s" -o build/xpass ./cmd/xpass

.PHONY: run
run:
	go run ./...

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
	install ./build/xpass /usr/local/bin
