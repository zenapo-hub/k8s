# syntax=docker/dockerfile:1

FROM --platform=$BUILDPLATFORM golang:1.24 as builder
ARG TARGETOS
ARG TARGETARCH
WORKDIR /workspace

COPY go.mod go.sum ./
RUN go mod download

COPY . .

RUN CGO_ENABLED=0 GOOS=$TARGETOS GOARCH=$TARGETARCH go build -o /workspace/bin/hpalogger ./cmd/hpalogger

FROM gcr.io/distroless/base-debian12
COPY --from=builder /workspace/bin/hpalogger /usr/local/bin/hpalogger
ENTRYPOINT ["/usr/local/bin/hpalogger"]
