.PHONY: build test lint fmt install clean

build:
	go build -o tf-plan-summary .

test:
	go test -race ./...

fmt:
	gofmt -w .

lint:
	gofmt -l . && go vet ./...

install:
	go install .

clean:
	rm -f tf-plan-summary
