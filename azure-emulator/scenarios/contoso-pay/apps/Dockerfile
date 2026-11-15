# Contoso Pay demo services: one image, SERVICE picks the process.
FROM node:22-alpine
WORKDIR /app
ENV NODE_ENV=production PORT=8080
COPY package.json package-lock.json ./
RUN npm ci --omit=dev --no-audit --no-fund && npm cache clean --force
COPY src ./src
COPY public ./public
USER 65532:65532
EXPOSE 8080
CMD ["node", "--require", "./src/otel.js", "./src/main.js"]
