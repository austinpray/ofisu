FROM golang:1.15.6
WORKDIR /usr/local/ofisu
COPY go.mod go.sum ./
RUN go mod download
COPY . .
# Avoid multi-stage build for now
# - we can use the -race flag
# - run tests in prod container
# - dev inside prod container
ARG GOFLAGS
RUN CGO_ENABLED=1 GOOS=linux GOARCH=amd64 go build ${GOFLAGS} -o /go/bin/ofisu
