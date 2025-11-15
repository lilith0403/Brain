# Estágio 1: Build (construção) da aplicação
FROM node:20-alpine AS builder
WORKDIR /app

# Instala Python e ferramentas de build necessárias para tree-sitter
RUN apk add --no-cache python3 make g++ build-base

# Copia os arquivos de definição de dependência
COPY package.json yarn.lock* package-lock.json* pnpm-lock.yaml* ./

# Instala as dependências (com a flag --legacy-peer-deps)
RUN \
  if [ -f yarn.lock ]; then yarn --frozen-lockfile; \
  elif [ -f package-lock.json ]; then npm ci --legacy-peer-deps; \
  elif [ -f pnpm-lock.yaml ]; then pnpm install --frozen-lockfile; \
  else echo "No lock file found, attempting npm install"; npm install --legacy-peer-deps; fi

# Copia todo o código-fonte
COPY . .

# Builda a aplicação NestJS
RUN npm run build

# Estágio 2: Produção (execução)
FROM node:20-alpine
WORKDIR /app

# Copia package.json e node_modules já compilados do builder
# Isso evita ter que recompilar dependências nativas como tree-sitter
COPY --from=builder /app/package.json ./
COPY --from=builder /app/node_modules ./node_modules

# Copia os arquivos compilados da aplicação
COPY --from=builder /app/dist ./dist

EXPOSE 3000
CMD ["node", "dist/main"]
