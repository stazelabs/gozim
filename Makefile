.PHONY: test test-race bench vet lint fuzz snapshot build clean testdata cover cover-html

test:
	go test ./zim/ -count=1
	cd cmd && go test ./... -count=1

test-race:
	go test -race ./zim/ -count=1
	cd cmd && go test -race ./... -count=1

bench:
	go test -bench=. -benchmem ./zim/

vet:
	go vet ./zim/
	cd cmd && go vet ./...

lint:
	golangci-lint run ./zim/
	cd cmd && golangci-lint run ./...

fuzz:
	go test -fuzz=FuzzParseHeader         -fuzztime=30s ./zim/
	go test -fuzz=FuzzParseDirectoryEntry -fuzztime=30s ./zim/
	go test -fuzz=FuzzParseMIMEList       -fuzztime=30s ./zim/
	go test -fuzz=FuzzExtractBlobs        -fuzztime=30s ./zim/

snapshot:
	goreleaser release --snapshot --clean

build:
	cd cmd && go build -o ../ziminfo   ./ziminfo/
	cd cmd && go build -o ../zimcat    ./zimcat/
	cd cmd && go build -o ../zimserve  ./zimserve/
	cd cmd && go build -o ../zimsearch ./zimsearch/
	cd cmd && go build -o ../zimverify ./zimverify/

clean:
	rm -f ziminfo zimcat zimserve zimsearch zimverify

cover:
	go test -coverprofile=coverage.out -covermode=atomic ./zim/
	go tool cover -func=coverage.out

cover-html: cover
	go tool cover -html=coverage.out

testdata: testdata/small.zim

testdata/small.zim:
	@mkdir -p testdata
	curl -sL "https://github.com/openzim/zim-testing-suite/raw/main/data/nons/small.zim" -o $@
