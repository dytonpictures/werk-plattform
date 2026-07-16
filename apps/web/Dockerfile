# syntax=docker/dockerfile:1.7
ARG NODE_VERSION=24.18.0
FROM node:${NODE_VERSION}-alpine AS deps
WORKDIR /workspace
COPY package.json package-lock.json ./
COPY apps/web/package.json apps/web/package.json
RUN npm ci

FROM deps AS build
COPY apps/web apps/web
RUN npm run build --workspace @werk/web

FROM node:${NODE_VERSION}-alpine AS runtime
ENV NODE_ENV=production
WORKDIR /app
RUN addgroup -S werk && adduser -S -G werk -u 10001 werk
COPY --from=build --chown=werk:werk /workspace/apps/web/.next/standalone ./
COPY --from=build --chown=werk:werk /workspace/apps/web/.next/static ./apps/web/.next/static
COPY --from=build --chown=werk:werk /workspace/apps/web/public ./apps/web/public
USER 10001:10001
EXPOSE 3000
CMD ["node", "apps/web/server.js"]
