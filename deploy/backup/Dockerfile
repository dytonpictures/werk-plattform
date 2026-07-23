FROM postgres:18-alpine
ARG VERSION=dev

ENV WERK_BUILD_VERSION=${VERSION}
LABEL org.opencontainers.image.version=${VERSION}

RUN apk add --no-cache age

COPY deploy/backup/werk-backup /usr/local/bin/werk-backup
RUN chmod 0555 /usr/local/bin/werk-backup

USER postgres
ENTRYPOINT ["/usr/local/bin/werk-backup"]
