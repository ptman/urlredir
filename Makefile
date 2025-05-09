# Copyright © Paul Tötterman <paul.totterman@gmail.com>. All rights reserved.

.PHONY: all
all: test urlredir

urlredir: main.go storage.go templates.go handlers.go errors.go
	CGO_ENABLED=0 go build -tags netgo,osusergo,timetzdata \
		    -trimpath -ldflags '-s -w' -o $@

.PHONY: run
run: urlredir
	./urlredir

.PHONY: test
test:
	CGO_ENABLED=0 go test -shuffle=on -vet=all -v

.PHONY: cover
cover:
	CGO_ENABLED=0 go test -coverprofile cover.out
	CGO_ENABLED=0 go tool cover -html cover.out

.PHONY: lint
lint:
	CGO_ENABLED=0 go tool golangci-lint run --default all \
		    --disable varnamelen,depguard

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

.PHONY: cleanschemas
cleanschemas:
	@psql urlredir -c '\t on' -c '\timing off' -c "SELECT schema_name FROM information_schema.schemata where schema_name ILIKE 'test%%'" | tail -n +2 |while read -r schema; do if [ -z "$${schema}" ]; then continue; fi; psql urlredir -c "DROP SCHEMA \"$${schema}\" CASCADE"; done
