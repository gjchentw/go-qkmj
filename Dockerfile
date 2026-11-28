# syntax=docker/dockerfile:1

FROM golang:1.26-alpine AS builder

WORKDIR /src

COPY go.mod go.sum ./
RUN go mod download

COPY cmd ./cmd
COPY internal ./internal

RUN CGO_ENABLED=0 GOOS=linux go build -trimpath -ldflags="-s -w" -o /out/qkmj ./cmd/qkmj

FROM gcr.io/distroless/static-debian12:nonroot

COPY --from=builder /out/qkmj /qkmj

EXPOSE 8081

USER nonroot:nonroot

ENTRYPOINT ["/qkmj"]
