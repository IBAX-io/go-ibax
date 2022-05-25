
# install tools
pb-install:
	go install google.golang.org/grpc/cmd/protoc-gen-go-grpc@latest
	go install github.com/golang/protobuf/protoc-gen-go@latest
	go get -u google.golang.org/grpc
	go install github.com/grpc-ecosystem/grpc-gateway/protoc-gen-grpc-gateway@latest
	go install github.com/grpc-ecosystem/grpc-gateway/protoc-gen-swagger@latest
	go install github.com/gogo/protobuf/protoc-gen-gogofaster@latest

# generate *.pb.go file
pb-generate:
	protoc -I=. --gogofaster_out=:../script --gogofaster_opt=paths=source_relative ./vm.proto
	protoc -I=. --gogofaster_out=:../smart --gogofaster_opt=paths=source_relative ./gas.proto
	protoc -I=. --gogofaster_out=:../types --gogofaster_opt=paths=source_relative ./block.proto
	protoc -I=.  --gogofaster_out=:../pbgo --gogofaster_opt=paths=source_relative ./tx.proto
	protoc -I=.  --gogofaster_out=:../common/crypto --gogofaster_opt=paths=source_relative ./crypto.proto
	protoc -I=.  --gogofaster_out=:../types --gogofaster_opt=paths=source_relative ./play.proto

