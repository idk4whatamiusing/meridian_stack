# build - context must be the monorepo root: bun's frozen lockfile resolves
# every workspace member (ui, packages) and the build reads the root
# package.json/bun.lock
FROM oven/bun:1-alpine AS build
WORKDIR /app
COPY package.json bun.lock* ./
COPY . .
RUN bun install --frozen-lockfile
RUN bun --cwd ui build
# runtime
FROM oven/bun:1-alpine
WORKDIR /app
COPY --from=build /app/ui/.next/standalone ./
COPY --from=build /app/ui/.next/static ./ui/.next/static
EXPOSE 3000
CMD ["node", "ui/server.js"]
