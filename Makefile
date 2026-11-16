.PHONY: build test smoke check install
build:
	go build -trimpath -o bin/subscale ./cmd/subscale
test:
	go test -race ./...
smoke: build
	python3 scripts/smoke_test.py
check:
	go vet ./...
	@test -z "$$(gofmt -l cmd internal)"
install:
	./scripts/install-source.sh
