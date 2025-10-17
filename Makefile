.PHONY: build
build:
	mkdir -p build
	go build -ldflags "-s" -o build/xpass ./cmd/xpass

.PHONY: run
run: build
	./build/xpass

.PHONY: clean
clean:
	rm -rf build
