FROM node:24-alpine
ARG VERSION=dev
WORKDIR /app
ENV WERK_DASHBOARD_ADDRESS=0.0.0.0:3000
ENV WERK_BUILD_VERSION=${VERSION}
LABEL org.opencontainers.image.version=${VERSION}
COPY server.mjs ./
COPY public ./public
USER node
EXPOSE 3000
CMD ["node", "server.mjs"]
