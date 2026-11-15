# syntax=docker/dockerfile:1
# CloudTyke dashboard: Next.js standalone server, non-root.
FROM node:24-alpine AS deps
WORKDIR /app
COPY package.json package-lock.json ./
RUN npm ci --no-audit --no-fund

FROM node:24-alpine AS build
WORKDIR /app
ENV NEXT_TELEMETRY_DISABLED=1
COPY --from=deps /app/node_modules ./node_modules
COPY . .
RUN npm run build

FROM node:24-alpine
LABEL org.opencontainers.image.title="CloudTyke dashboard" \
      org.opencontainers.image.description="Web console for the CloudTyke local Azure emulator. Not affiliated with or endorsed by Microsoft." \
      org.opencontainers.image.source="https://github.com/bwalia/cloudtyke" \
      org.opencontainers.image.licenses="Apache-2.0"
WORKDIR /app
ENV NODE_ENV=production NEXT_TELEMETRY_DISABLED=1 HOSTNAME=0.0.0.0 PORT=4310
COPY --from=build --chown=node:node /app/.next/standalone ./
COPY --from=build --chown=node:node /app/.next/static ./.next/static
USER node
EXPOSE 4310
HEALTHCHECK --interval=10s --timeout=3s --start-period=10s --retries=5 CMD ["node", "-e", "fetch('http://127.0.0.1:4310/').then(r => process.exit(r.status < 500 ? 0 : 1)).catch(() => process.exit(1))"]
CMD ["node", "server.js"]
