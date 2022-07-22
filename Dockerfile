FROM golang:1.18 as builder
RUN go env -w GO111MODULE=on
RUN go env -w GOPROXY=https://goproxy.cn,direct

WORKDIR /go/src/go-ibax
COPY . .
RUN make

FROM golang:1.18

COPY --from=builder /go/src/go-ibax/go-ibax /mnt/ibax/

ENTRYPOINT sh /mnt/ibax/data/ibax-startup.sh
