.PHONY: build
build:
	go build -o tview-tests ./cmd/tview-tests

.PHONY: update
update:
	GOPROXY= GOSUMDB= go get -u -v ./...
	go mod tidy
	go mod vendor

.PHONY: test
test:
	go test ./...

.PHONY: vet
vet:
	go vet ./...

.PHONY: clean
clean:
	rm -f tview-tests
