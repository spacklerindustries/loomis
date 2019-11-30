DIR := $(PWD)
GOCMD=go

ARTIFACT_NAME=lagoon
ARTIFACT_DESTINATION=$(GOPATH)/bin

PKG=github.com/spacklerindustries/loomis
PKGMODPATH=$(DIR)/vendor

VERSION=$(shell git rev-parse HEAD)
BUILD=$(shell date +%FT%T%z)

# LDFLAGS=-ldflags "-w -s -X ${PKG}/version=${VERSION} -X ${PKG}/build=${BUILD}"
LDFLAGS=

GOLANG_IMAGE=golang:1.12
GOLANG_IMAGE_ARM=arm32v7/golang:1.12

all: deps test build

all-docker-linux: 
	deps-docker
	test-docker-linux
	build-docker-linux

all-docker-arm: 
	deps-docker-arm
	test-docker-arm
	build-docker-arm

deps:
	GO111MODULE=on ${GOCMD} get -v
test:
	GO111MODULE=on $(GOCMD) fmt ./...
	GO111MODULE=on $(GOCMD) vet ./...
	GO111MODULE=on $(GOCMD) test -v ./...

clean:
	$(GOCMD) clean

build:
	GO111MODULE=on $(GOCMD) build ${LDFLAGS} -o ${ARTIFACT_DESTINATION}/${ARTIFACT_NAME} -v
build-linux:
	GO111MODULE=on GOOS=linux GOARCH=amd64 $(GOCMD) build ${LDFLAGS} -o builds/loomis-${VERSION}-linux-amd64 -v
build-arm:
	GO111MODULE=on GOOS=linux GOARCH=arm $(GOCMD) build ${LDFLAGS} -o builds/loomis-${VERSION}-arm -v

## build using docker golang
deps-docker:
	docker run \
	-v $(PKGMODPATH):/go/pkg/mod \
	-v $(DIR):/go/src/${PKG}/ \
	-e GO111MODULE=on \
	-w="/go/src/${PKG}/" \
	${GOLANG_IMAGE} go get -v

## build using docker golang
deps-docker-arm:
	docker run \
	-v $(PKGMODPATH):/go/pkg/mod \
	-v $(DIR):/go/src/${PKG}/ \
	-e GO111MODULE=on \
	-w="/go/src/${PKG}/" \
	${GOLANG_IMAGE_ARM} go get -v

## build using docker golang
test-docker:
	docker run \
	-v $(PKGMODPATH):/go/pkg/mod \
	-v $(DIR):/go/src/${PKG}/ \
	-e GO111MODULE=on \
	-e GOOS=linux \
	-e GOARCH=amd64 \
	-w="/go/src/${PKG}/" \
	${GOLANG_IMAGE} /bin/bash -c " \
	go fmt ./... && \
	go vet ./... && \
	go test -v ./..."

## build using docker golang
test-docker-arm:
	docker run \
	-v $(PKGMODPATH):/go/pkg/mod \
	-v $(DIR):/go/src/${PKG}/ \
	-e GO111MODULE=on \
	-e GOOS=linux \
	-e GOARCH=arm \
	-w="/go/src/${PKG}/" \
	${GOLANG_IMAGE_ARM} /bin/bash -c " \
	go fmt ./... && \
	go vet ./... && \
	go test -v ./..."

## build using docker golang
build-docker-linux:
	docker run \
	-v $(PKGMODPATH):/go/pkg/mod \
	-v $(DIR):/go/src/${PKG}/ \
	-e GO111MODULE=on \
	-e GOOS=linux \
	-e GOARCH=amd64 \
	-w="/go/src/${PKG}/" \
	${GOLANG_IMAGE} go build ${LDFLAGS} -o builds/loomis-${VERSION}-linux-amd64

## build using docker golang
build-docker-linux-arm:
	docker run \
	-v $(PKGMODPATH):/go/pkg/mod \
	-v $(DIR):/go/src/${PKG}/ \
	-e GO111MODULE=on \
	-e GOOS=linux \
	-e GOARCH=arm \
	-w="/go/src/${PKG}/" \
	${GOLANG_IMAGE_ARM} go build ${LDFLAGS} -o builds/loomis-${VERSION}-arm

install-linux:
	cp builds/loomis-${VERSION}-linux-amd64 ${ARTIFACT_DESTINATION}/loomis
install-arm:
	cp builds/loomis-${VERSION}-arm ${ARTIFACT_DESTINATION}/loomis