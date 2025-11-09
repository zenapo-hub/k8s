# syntax=docker/dockerfile:1.4

FROM golang:1.24 AS builder
ARG TARGETOS
ARG TARGETARCH
ARG TARGETVARIANT
ARG TARGET_BINARY=hpalogger
WORKDIR /workspace

COPY go.mod go.sum ./
RUN go mod download

COPY . .

ENV CGO_ENABLED=0
RUN GOOS=${TARGETOS:-linux} GOARCH=${TARGETARCH:-amd64} \
    go build -o "/workspace/bin/${TARGET_BINARY}" "./cmd/${TARGET_BINARY}"

FROM gcr.io/distroless/base-debian12
ARG TARGET_BINARY=hpalogger
COPY --from=builder "/workspace/bin/${TARGET_BINARY}" "/usr/local/bin/app"
ENTRYPOINT ["/usr/local/bin/app"]
