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
	cd internal/types; go generate

TESTPKGS=github.com/mikeb26/gptcli/cmd/gptcli github.com/mikeb26/gptcli/internal github.com/mikeb26/gptcli/internal/prompts github.com/mikeb26/gptcli/internal/ui github.com/mikeb26/gptcli/internal/am

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
	rm -f gptcli unit-tests.xml internal/types/openai_client_mock.go

.PHONY: deps
deps:
	rm -rf go.mod go.sum vendor
	go mod init github.com/mikeb26/gptcli
	go mod edit -replace=github.com/gbin/goncurses=github.com/mikeb26/gobin-goncurses@39be5170905868a7d76f9b21df4738b37b6adaf7
	GOPROXY=direct go mod tidy
	go mod vendor

FORCE:
