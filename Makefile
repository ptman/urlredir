# Copyright (c) 2017-2021 Paul Tötterman <ptman@iki.fi>. All rights reserved.
GIT_REV:=$(shell git describe --always --dirty)
REV_DATE:=$(shell go run tools/gitrevdate.go)
GC_FLAGS:=-trimpath $(GOPATH)/src
LD_FLAGS:=-s -w -X main.gitRev=$(GIT_REV) -X "main.revDateS=$(REV_DATE)"

.PHONY: all
all: test urlredir

urlredir: main.go storage.go templates.go handlers.go
	CGO_ENABLED=0 go build -gcflags '$(GC_FLAGS)' -ldflags '$(LD_FLAGS)' -o $@ $^

.PHONY: run
run: urlredir
	./urlredir

.PHONY: test
test:
	CGO_ENABLED=0 go test -short

.PHONY: testall
testall:
	CGO_ENABLED=0 go test

.PHONY: cover
cover:
	CGO_ENABLED=0 go test -coverprofile cover.out
	CGO_ENABLED=0 go tool cover -html cover.out

.PHONY: lint
lint:
	CGO_ENABLED=0 golangci-lint run --enable-all --disable paralleltest,testpackage,exhaustivestruct,forbidigo

.PHONY: cloc
cloc:
	@cloc .
	@echo only tests
	@cloc *_test.go

.PHONY: docker
docker:
	docker build -t urlredir .

.PHONY: clean
clean:
	rm -f urlredir cover.out
