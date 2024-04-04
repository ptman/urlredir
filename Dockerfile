# Copyright © Paul Tötterman <paul.totterman@gmail.com>. All rights reserved.
FROM golang:1.22-alpine3.19 AS builder

ENV CGO_ENABLED=0
COPY . ${GOPATH}/urlredir
WORKDIR ${GOPATH}/urlredir
RUN go build

FROM scratch
LABEL author="Paul Tötterman <paul.totterman@gmail.com>"

COPY --from=builder /go/urlredir/urlredir /

EXPOSE 8080
CMD ["/urlredir"]
