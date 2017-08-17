# Copyright (c) 2017 Paul Tötterman <ptman@iki.fi>. All rights reserved.
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
	go test -short

.PHONY: testall
testall:
	go test

.PHONY: cover
cover:
	go test -coverprofile cover.out
	go tool cover -html cover.out

.PHONY: lint
lint:
	gometalinter

.PHONY: cloc
cloc:
	@echo excluding vendor
	@cloc --exclude-dir=vendor .
	@echo only tests
	@cloc *_test.go

.PHONY: docker
docker: urlredir
	docker build -t urlredir .

.PHONY: clean
clean:
	rm -f urlredir cover.out
