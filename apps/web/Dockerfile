# apps/web/Dockerfile
# Efficient SvelteKit production image
FROM node:20-alpine AS builder

WORKDIR /app

# Enable pnpm
RUN corepack enable && corepack prepare pnpm@9 --activate

COPY package.json pnpm-lock.yaml* ./
RUN pnpm install

COPY . .

RUN pnpm build

# Runtime
FROM node:20-alpine

WORKDIR /app

RUN corepack enable && corepack prepare pnpm@9 --activate

COPY --from=builder /app/package.json /app/pnpm-lock.yaml* ./
COPY --from=builder /app/node_modules ./node_modules
COPY --from=builder /app/build ./build
COPY --from=builder /app/.svelte-kit ./.svelte-kit

EXPOSE 3000

ENV PORT=3000
ENV HOST=0.0.0.0

CMD ["pnpm", "preview"]
