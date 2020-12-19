FROM debian:10.7-slim as base

ENV DEBIAN_FRONTEND noninteractive

WORKDIR /usr/local/ofisu

RUN apt-get update \
 && apt-get install -y \
    ca-certificates \
    graphviz=2.40.1-6 \
 && rm -rf /var/lib/apt/lists/*

FROM golang:1.15.6 as builder
WORKDIR /usr/local/ofisu
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -o /go/bin/ofisu

FROM base
COPY --from=builder /go/bin/ofisu /usr/local/bin
