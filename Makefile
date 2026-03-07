.PHONY: test test-race bench vet build clean testdata

test:
	go test ./... -count=1

test-race:
	go test -race ./... -count=1

bench:
	go test -bench=. -benchmem ./zim/

vet:
	go vet ./...

build:
	go build ./cmd/ziminfo/
	go build ./cmd/zimcat/
	go build ./cmd/zimserve/
	go build ./cmd/zimsearch/
	go build ./cmd/zimverify/

clean:
	rm -f ziminfo zimcat zimserve zimsearch zimverify

testdata: testdata/small.zim

testdata/small.zim:
	@mkdir -p testdata
	curl -sL "https://github.com/openzim/zim-testing-suite/raw/main/data/nons/small.zim" -o $@
