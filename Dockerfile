ARG SERVICE=api
ARG VERSION=dev
FROM golang:1.26-alpine AS build
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
ARG SERVICE
RUN CGO_ENABLED=0 go build -trimpath -ldflags="-s -w" -o /out/werk ./cmd/${SERVICE}

FROM build AS test
ENV GOCACHE=/tmp/go-build
ENTRYPOINT ["go", "test"]

FROM alpine:3.22
ARG VERSION
ENV WERK_BUILD_VERSION=${VERSION}
LABEL org.opencontainers.image.version=${VERSION}
RUN apk add --no-cache ca-certificates tzdata \
    && addgroup -S werk \
    && adduser -S -G werk werk
COPY --from=build /out/werk /usr/local/bin/werk
USER werk
ENTRYPOINT ["/usr/local/bin/werk"]
