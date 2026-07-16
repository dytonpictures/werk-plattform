# syntax=docker/dockerfile:1.7
ARG GO_VERSION=1.26.1
FROM golang:${GO_VERSION}-alpine AS build
WORKDIR /src
COPY go.mod go.sum* ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 GOOS=linux go build -trimpath -ldflags="-s -w" -o /out/werk-api ./cmd/server

FROM alpine:3.23
RUN addgroup -S werk && adduser -S -G werk -u 10001 werk
WORKDIR /app
COPY --from=build --chown=werk:werk /out/werk-api /app/werk-api
COPY --chown=werk:werk migrations /app/migrations
USER 10001:10001
EXPOSE 8080
ENTRYPOINT ["/app/werk-api"]
