.PHONY: all test clean

all: test build

build: test
	mkdir -p build
	go build -o build/autocannon-go ./main.go

test:
	go test ./...

clean:
	go clean ./...
