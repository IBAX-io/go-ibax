#!/bin/bash

# install protoc 3.7.1
# export GO111MODULES=on
# go install google.golang.org/protobuf/cmd/protoc-gen-go@latest
# go install github.com/grpc-ecosystem/grpc-gateway/v2/protoc-gen-grpc-gateway@latest

protoc -I ./ ./*.proto \
    -I ./googleapis \
    --go_out ./gengo/ --go_opt paths=source_relative \
    --go-grpc_out ./gengo/ --go-grpc_opt paths=source_relative \
    --grpc-gateway_out ./gengo/ --grpc-gateway_opt paths=source_relative

