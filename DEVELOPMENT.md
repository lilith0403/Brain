# 🛠️ Guia de Desenvolvimento Local

Este guia explica como rodar a API localmente enquanto os outros serviços (Redis, ChromaDB, Watcher) rodam no Docker Compose.

## 📋 Pré-requisitos

- Node.js e npm instalados (para rodar a API localmente)
- Docker e Docker Compose instalados
- Go instalado (para compilar a CLI)

## 🚀 Configuração Inicial

### 1. Configure o arquivo `.env`

Copie o arquivo `env.example` para `.env`:

```bash
cp env.example .env
```

Edite o `.env` e configure com os caminhos do **SEU sistema**:

```ini
# Sua chave da API Google Gemini
GOOGLE_API_KEY=sua_chave_aqui

# IMPORTANTE: Configure estes caminhos com valores absolutos do seu sistema
# Caminho para o arquivo de scan paths
SCAN_PATHS_FILE=/home/seu_usuario/.config/brain/scan.paths

# Caminho base para montar no Docker (geralmente seu home)
SCAN_PATHS_MOUNT=/home/seu_usuario
```

**⚠️ Atenção:** Use caminhos **absolutos**, não use `~/` no `.env` do docker-compose.

### 2. Crie o arquivo de scan paths

Crie o arquivo definido em `SCAN_PATHS_FILE` e adicione os caminhos que deseja indexar:

```bash
mkdir -p ~/.config/brain
nano ~/.config/brain/scan.paths
```

Exemplo de conteúdo:

```ini
# ~/.config/brain/scan.paths

# Meus dotfiles
/home/seu_usuario/.config/hypr
/home/seu_usuario/.config/waybar
/home/seu_usuario/.zshrc

# Meus repositórios
/home/seu_usuario/Repos
```

**Nota:** Dentro do arquivo `scan.paths`, você **pode** usar `~/` nos caminhos - ele será expandido automaticamente pelo watcher. Mas no `.env`, use sempre caminhos absolutos.

## 🔧 Desenvolvimento Local da API

### Método 1: Parar o container da API e rodar localmente

1. **Suba apenas os serviços necessários (sem a API):**

```bash
docker compose up -d db queue watcher
```

Isso inicia:
- Redis (porta 6379)
- ChromaDB (porta 8000)
- Watcher (monitora seus arquivos)

2. **Instale as dependências da API:**

```bash
cd brain-api
npm install
```

3. **Configure as variáveis de ambiente para desenvolvimento local:**

Crie um arquivo `.env` dentro de `brain-api/` ou exporte as variáveis:

```bash
export GOOGLE_API_KEY=sua_chave_aqui
export REDIS_HOST=localhost
export REDIS_PORT=6379
export CHROMA_HOST=localhost
export CHROMA_PORT=8000
```

Ou adicione ao `.env` na raiz do projeto (será lido automaticamente pelo NestJS ConfigModule).

4. **Rode a API localmente em modo de desenvolvimento:**

```bash
cd brain-api
npm run start:dev
```

A API agora estará rodando em `http://localhost:3000` e se conectará aos serviços no Docker!

### Método 2: Usar o docker-compose sem a API

Se preferir, você pode comentar o serviço `api` no `docker-compose.yml` e sempre rodar apenas os serviços necessários:

```yaml
# Comente estas linhas para desabilitar a API no Docker
# api:
#   ...
```

Então sempre use:

```bash
docker compose up -d db queue watcher
```

## 🔍 Verificando se está funcionando

1. **Verifique os logs do watcher:**

```bash
docker compose logs -f watcher
```

Você deve ver mensagens de scan inicial.

2. **Verifique os logs da API local:**

Se rodou a API localmente, você verá logs no terminal onde executou `npm run start:dev`.

3. **Teste a API:**

```bash
curl -X POST http://localhost:3000/ai/ask \
  -H "Content-Type: application/json" \
  -d '{"query": "teste"}'
```

## 🐛 Debugging

### Problema: API local não consegue conectar ao Redis/ChromaDB

**Solução:** Certifique-se de que:
- Os serviços estão rodando: `docker compose ps`
- As variáveis de ambiente estão corretas (`REDIS_HOST=localhost`, `CHROMA_HOST=localhost`)
- As portas não estão bloqueadas: `netstat -tuln | grep -E '6379|8000'`

### Problema: Watcher não encontra os arquivos

**Solução:** 
- Verifique se `SCAN_PATHS_FILE` no `.env` aponta para o arquivo correto
- Verifique se `SCAN_PATHS_MOUNT` está montando o diretório correto no Docker
- Veja os logs: `docker compose logs watcher`

### Problema: Watcher não consegue conectar à API local

**Solução:**
- Certifique-se de que `API_URL` no `.env` está configurado como `http://host.docker.internal:3000/queue/ingest`
- Verifique se a API local está rodando em `0.0.0.0:3000` (não apenas `localhost`)
- No Linux, você pode precisar usar `host.docker.internal` ou configurar o Docker network

## 📝 Variáveis de Ambiente Importantes

| Variável | Descrição | Obrigatório | Exemplo |
|----------|-----------|-------------|---------|
| `GOOGLE_API_KEY` | Chave da API do Google Gemini | ✅ Sim | `AIzaSy...` |
| `SCAN_PATHS_FILE` | Caminho **absoluto** para o arquivo de scan paths | ✅ Sim | `/home/usuario/.config/brain/scan.paths` |
| `SCAN_PATHS_MOUNT` | Caminho **absoluto** base para montar no Docker | ✅ Sim | `/home/usuario` |
| `API_URL` | URL da API para o watcher | ❌ Não | `http://host.docker.internal:3000/queue/ingest` |
| `REDIS_HOST` | Host do Redis (API local) | ❌ Não | `localhost` |
| `CHROMA_HOST` | Host do ChromaDB (API local) | ❌ Não | `localhost` |
| `USER_HOME` | Home do usuário (auto-detectado se não definido) | ❌ Não | `/home/usuario` |

## 🎯 Workflow de Desenvolvimento Recomendado

1. Configure o `.env` uma vez
2. Suba os serviços de infraestrutura: `docker compose up -d db queue watcher`
3. Rode a API localmente para desenvolvimento: `cd brain-api && npm run start:dev`
4. Faça suas alterações no código
5. A API recarrega automaticamente (hot reload)
6. Teste suas mudanças

Quando terminar o desenvolvimento, pode parar os serviços:

```bash
docker compose down
```

Para parar tudo e remover os volumes:

```bash
docker compose down -v
```

