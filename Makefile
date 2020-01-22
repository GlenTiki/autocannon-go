.PHONY: all test clean

all: test build

build: test
	mkdir -p build
	go build -o build/autocannon-go ./autocannon.go

test:
	go test ./...

clean:
	go clean ./...
