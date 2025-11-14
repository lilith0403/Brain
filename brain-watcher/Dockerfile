# Estágio 1: Build (construção) da aplicação Go
# CORREÇÃO: Use a versão 1.25 do Go
FROM golang:1.25-alpine AS builder

WORKDIR /app

# Copia os arquivos de módulo Go
COPY go.mod go.sum ./

# Baixa as dependências
RUN go mod download

# Copia todo o código-fonte da aplicação
COPY . .

# Compila a aplicação Go em um binário estático
RUN CGO_ENABLED=0 GOOS=linux go build -a -installsuffix cgo -o brain-watcher .

# Estágio 2: Produção (execução)
FROM alpine:latest
WORKDIR /root/

# Copia APENAS o binário compilado
COPY --from=builder /app/brain-watcher .

# (Vamos definir o comando para executar, mas as variáveis de ambiente
# e os volumes serão configurados no docker-compose)
CMD ["./brain-watcher"]
