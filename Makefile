export GOPROXY=https://goproxy.io
export GO111MODULE=on

HOMEDIR := $(shell pwd)

all: mod build

mod:
	go mod tidy -v

build:
	bash $(HOMEDIR)/build.sh

try:
	go build
	go-ibax generateFirstBlock --test=true
	go-ibax initDatabase
	go-ibax start

init:
	go-ibax initDatabase

# avoid filename conflict and speed up build
.PHONY: all
