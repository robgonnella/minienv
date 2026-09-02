FROM golang:1.27.1-alpine3.24 AS builder
WORKDIR /build
COPY cmd/hello/main.go main.go
RUN CGO_ENABLED=0 go build -ldflags="-s -w" -trimpath -o server main.go

FROM alpine:3.24
COPY --from=builder /build/server /server
CMD ["/server"]
