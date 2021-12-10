export GOPROXY=https://goproxy.io
export GO111MODULE=on

HOMEDIR := $(shell pwd)

all: mod build

mod:
	go mod tidy -v

build:
	bash $(HOMEDIR)/build.sh

# avoid filename conflict and speed up build
.PHONY: all
