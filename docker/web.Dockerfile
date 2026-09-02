FROM oven/bun:1.3.14 AS build
WORKDIR /app
COPY package.json bun.lock .npmrc ./
RUN bun install --frozen-lockfile
COPY . .
RUN bun run build

FROM oven/bun:1.3.14-slim AS runtime
WORKDIR /app
# dependencies is empty so adapter-node bundles everything into build/;
# if a package ever moves to "dependencies", add a production install here.
COPY --from=build --chown=bun:bun /app/build ./build
USER bun
ENV NODE_ENV=production PORT=3000
EXPOSE 3000
CMD ["bun", "./build/index.js"]
