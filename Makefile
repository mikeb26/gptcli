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

.PHONY: clean
clean:
	rm -f gptcli unit-tests.xml

.PHONY: deps
deps:
	rm -rf go.mod go.sum vendor
	go mod init github.com/mikeb26/gptcli
	GOPROXY=direct go mod tidy
	go mod vendor

FORCE:
