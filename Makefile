.PHONY: test test-race bench vet lint fuzz snapshot build clean testdata cover cover-html

test:
	go test ./... -count=1

test-race:
	go test -race ./... -count=1

bench:
	go test -bench=. -benchmem ./zim/

vet:
	go vet ./...

lint:
	golangci-lint run ./...

fuzz:
	go test -fuzz=FuzzParseHeader         -fuzztime=30s ./zim/
	go test -fuzz=FuzzParseDirectoryEntry -fuzztime=30s ./zim/
	go test -fuzz=FuzzParseMIMEList       -fuzztime=30s ./zim/
	go test -fuzz=FuzzExtractBlobs        -fuzztime=30s ./zim/

snapshot:
	goreleaser release --snapshot --clean

build:
	go build ./cmd/ziminfo/
	go build ./cmd/zimcat/
	go build ./cmd/zimserve/
	go build ./cmd/zimsearch/
	go build ./cmd/zimverify/

clean:
	rm -f ziminfo zimcat zimserve zimsearch zimverify

cover:
	go test -coverprofile=coverage.out -covermode=atomic ./...
	go tool cover -func=coverage.out

cover-html: cover
	go tool cover -html=coverage.out

testdata: testdata/small.zim

testdata/small.zim:
	@mkdir -p testdata
	curl -sL "https://github.com/openzim/zim-testing-suite/raw/main/data/nons/small.zim" -o $@
