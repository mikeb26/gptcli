export GO111MODULE=on
export GOFLAGS=-mod=vendor

.PHONY: build
build: cmd/gptcli

cmd/gptcli: vendor FORCE
	go build -o gptcli cmd/gptcli/*.go

vendor: go.mod
	go mod download
	go mod vendor

cmd/gptcli/version.txt:
	git describe --tags > cmd/gptcli/version.txt
	truncate -s -1 cmd/gptcli/version.txt

.PHONY: mocks
mocks:
	cd internal; go generate

TESTPKGS=github.com/mikeb26/gptcli/cmd/gptcli

.PHONY: test
test: mocks
	go test $(TESTPKGS)

unit-tests.xml: mocks FORCE
	gotestsum --junitfile unit-tests.xml $(TESTPKGS)

.PHONY: lint
lint:
	golangci-lint run ./...

.PHONY: clean
clean:
	rm -f gptcli unit-tests.xml internal/openai_client_mock.go

.PHONY: deps
deps:
	rm -rf go.mod go.sum vendor
	go mod init github.com/mikeb26/gptcli
	go mod edit -replace=github.com/cloudwego/eino-ext/components/model/gemini=github.com/mikeb26/cloudwego-eino-ext/components/model/gemini@v0.1.14
	GOPROXY=direct go mod tidy
	go mod vendor

FORCE:
