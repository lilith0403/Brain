package main

import (
	"bufio"
	"bytes"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/fsnotify/fsnotify"
	"github.com/joho/godotenv" // Importa o godotenv
)

// O "DTO" que a API NestJS espera
type IngestPayload struct {
	FilePath     string    `json:"filePath"`
	Content      string    `json:"content"`
	LastModified time.Time `json:"lastModified"`
}

// Cooldown para evitar envios duplicados
const cooldown = 10 * time.Second

var (
	mutex          = &sync.Mutex{}
	lastIngestTime = make(map[string]time.Time)
	homeDir        string // Salva o diretório Home
)

// --- LÓGICA DE FILTRO (HÍBRIDA) ---

// Pastas que NUNCA queremos descer, mesmo dentro de um projeto
var blocklistSubDirs = []string{
	".git",
	"node_modules",
	".cache",
	"__pycache__",
	"chroma_data",
	"build",
	"dist",
	"target",
	"go/pkg/mod",
	".config/notion-app-enhanced",
	".config/Cursor",
	".config/BraveSoftware",
	".config/discord",
	".config/spotify",
	".local/share/Steam",
	".local/share/Trash",
	".docker",
	".config/GitKraken",
	".config/MongoDB Compass",
	".config/Notion",
	".config/teams-for-linux",
	".config/Postman",
	".config/pulse",
	".config/vscode-vibrancy-continued-nodejs",
}

// Extensões de arquivo que NÓS QUEREMOS.
var whitelistExtensions = map[string]bool{
	".txt":  true,
	".md":   true,
	".conf": true,
	".sh":   true,
	".json": true,
	".js":   true,
	".ts":   true,
	".go":   true,
	".py":   true,
	".css":  true,
	".html": true,
	".toml": true,
	".yaml": true,
	".yml":  true,
}

// Nomes de arquivos exatos que QUEREMOS
var whitelistFilenames = map[string]bool{
	".zshrc":        true,
	".bashrc":       true,
	".bash_profile": true,
	".gitconfig":    true,
	"config":        true,
}

// shouldProcess checa se devemos processar este caminho.
func shouldProcess(path string, isDir bool) bool {
	// 1. Checar se está em uma subpasta bloqueada
	for _, blockedDir := range blocklistSubDirs {
		// Checa se o caminho CONTÉM a pasta bloqueada.
		if strings.Contains(path, "/"+blockedDir+"/") || strings.HasSuffix(path, "/"+blockedDir) {
			return false // Está em uma pasta bloqueada
		}
	}

	// 2. Se for um arquivo...
	if !isDir {
		filename := filepath.Base(path)
		// 2a. Checa se o NOME EXATO está na whitelist
		if _, ok := whitelistFilenames[filename]; ok {
			return true // Ex: .zshrc
		}

		// 2b. Se não, checa se a EXTENSÃO está na whitelist
		ext := filepath.Ext(path)
		if _, ok := whitelistExtensions[ext]; ok {
			return true // Ex: main.go
		}

		return false // Não é um nome ou extensão que queremos
	}

	// 3. Se for um diretório que não foi bloqueado, nós o varremos
	return true
}

// Lê o arquivo de configuração (no caminho especificado) e retorna a lista de caminhos
func loadPathsToScan(configPath string) ([]string, error) {
	// Expande o '~/' no caminho do *arquivo de configuração* (para o .env local)
	if strings.HasPrefix(configPath, "~/") {
		configPath = filepath.Join(homeDir, configPath[2:])
	}

	file, err := os.Open(configPath)
	if err != nil {
		if os.IsNotExist(err) {
			log.Printf("AVISO: Arquivo de configuração não encontrado em: %s", configPath)
			return []string{}, nil
		}
		return nil, err
	}
	defer file.Close()

	var paths []string
	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line != "" && !strings.HasPrefix(line, "#") {
			// Expande '~/' nos caminhos DENTRO do arquivo
			if strings.HasPrefix(line, "~/") {
				line = filepath.Join(homeDir, line[2:])
			}
			paths = append(paths, line)
		}
	}
	return paths, scanner.Err()
}

func main() {
	// Tenta carregar o .env local (para 'go run .')
	err := godotenv.Load()
	if err != nil && !os.IsNotExist(err) {
		log.Printf("AVISO: Erro ao carregar .env local: %v", err)
	}

	watcher, err := fsnotify.NewWatcher()
	if err != nil {
		log.Fatal(err)
	}
	defer watcher.Close()

	// Salva o diretório Home globalmente (para expandir '~')
	// Primeiro tenta usar USER_HOME da variável de ambiente (útil no Docker)
	if userHome := os.Getenv("USER_HOME"); userHome != "" {
		homeDir = userHome
		log.Printf("Usando USER_HOME do ambiente: %s", homeDir)
	} else {
		// Fallback para o home do sistema
		homeDir, err = os.UserHomeDir()
		if err != nil {
			log.Fatal("Erro ao encontrar o diretório home:", err)
		}
		log.Printf("Usando home do sistema: %s", homeDir)
	}

	// 1. Lê o *caminho* do arquivo de configuração do ambiente
	configFile := os.Getenv("SCAN_PATHS_FILE")
	if configFile == "" {
		log.Fatal("ERRO: Variável de ambiente SCAN_PATHS_FILE não definida.")
	}

	// 2. Carrega os caminhos DELE
	pathsToScan, err := loadPathsToScan(configFile)
	if err != nil {
		log.Fatalf("Erro ao ler o arquivo de configuração (%s): %v", configFile, err)
	}

	if len(pathsToScan) == 0 {
		log.Println("Nenhum caminho de scan definido. O scan inicial será pulado.")
	} else {
		log.Printf("Iniciando Scan Inicial (Fase 1) em %d caminhos...", len(pathsToScan))
	}

	// Loop por CADA caminho que queremos escanear
	for _, path := range pathsToScan {
		absPath, err := filepath.Abs(path) // Garante que o caminho é absoluto
		if err != nil {
			log.Printf("ERRO: Caminho inválido %s: %v", path, err)
			continue
		}
		log.Printf("Iniciando varredura em: %s\n", absPath)

		err = filepath.Walk(absPath, func(walkPath string, info os.FileInfo, err error) error {
			if err != nil {
				if os.IsPermission(err) {
					return filepath.SkipDir
				}
				log.Printf("ERRO no scan: %v\n", err)
				return err
			}

			if !shouldProcess(walkPath, info.IsDir()) {
				if info.IsDir() {
					log.Printf("IGNORANDO (Scan-Dir): %s\n", walkPath)
					return filepath.SkipDir // Pula este diretório
				}
				return nil // Apenas pula este arquivo
			}

			if info.IsDir() {
				err = watcher.Add(walkPath)
				if err != nil && !os.IsPermission(err) {
					log.Printf("ERRO ao adicionar %s ao watcher: %v\n", walkPath, err)
				}
			}

			if !info.IsDir() {
				log.Printf("INDEXANDO (Scan): %s\n", walkPath)
				go ingestFile(walkPath)
			}
			return nil
		})

		if err != nil {
			log.Printf("ERRO FATAL no Scan de %s: %v", path, err)
		}
	}

	fmt.Println("Scan Inicial (Fase 1) concluído.")
	fmt.Println("Brain Watcher iniciado. (Fase 2: Monitorando mudanças)...")

	// Loop de monitoramento (Fase 2)
	for {
		select {
		case event, ok := <-watcher.Events:
			if !ok {
				return
			}
			if shouldProcess(event.Name, false) {
				if event.Op == fsnotify.Create || event.Op == fsnotify.Write {
					log.Println("AÇÃO:", event.Op, event.Name)
					go ingestFile(event.Name)
				}
			}

		case err, ok := <-watcher.Errors:
			if !ok {
				return
			}
			log.Println("ERRO:", err)
		}
	}
}

// ingestFile cuida do debounce, leitura, FILTRO 2 e envio
func ingestFile(filePath string) {
	// --- Bloco de Debounce (Cooldown) ---
	mutex.Lock()
	if lastTime, found := lastIngestTime[filePath]; found {
		if time.Since(lastTime) < cooldown {
			mutex.Unlock()
			return
		}
	}
	lastIngestTime[filePath] = time.Now()
	mutex.Unlock()
	// --- Fim do Debounce ---

	time.Sleep(100 * time.Millisecond)

	fileInfo, err := os.Stat(filePath)
	if err != nil {
		if !os.IsPermission(err) {
			// log.Printf("ERRO (stat): %v\n", err)
		}
		return
	}

	const maxSize = 25 * 1024 * 1024 // 25 MB
	if fileInfo.Size() > maxSize {
		log.Printf("IGNORANDO (arquivo gigante): %s\n", filePath)
		return
	}

	contentBytes, err := os.ReadFile(filePath)
	if err != nil {
		if !os.IsPermission(err) {
			// log.Printf("ERRO ao ler %s: %v\n", filePath, err)
		}
		return
	}

	if len(contentBytes) == 0 {
		return
	}

	// --- FILTRO 2: FILTRO DE CONTEÚDO (MIME Type) ---
	mimeType := http.DetectContentType(contentBytes)
	if !strings.HasPrefix(mimeType, "text/") {
		log.Printf("IGNORANDO (não-texto: %s): %s\n", mimeType, filePath)
		return
	}
	// --- FIM DO FILTRO 2 ---

	payload := IngestPayload{
		FilePath:     filePath,
		Content:      string(contentBytes),
		LastModified: fileInfo.ModTime(),
	}

	jsonPayload, err := json.Marshal(payload)
	if err != nil {
		log.Println("ERRO ao criar JSON:", err)
		return
	}

	// 5. Envie o HTTP POST para a API (lendo do .env)
	apiURL := os.Getenv("API_URL")
	if apiURL == "" {
		log.Fatal("ERRO: Variável de ambiente API_URL não definida.")
	}

	resp, err := http.Post(apiURL, "application/json", bytes.NewBuffer(jsonPayload))
	if err != nil {
		log.Println("ERRO ao enviar para a API:", err)
		return
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 200 && resp.StatusCode < 300 {
		log.Printf("SUCESSO: Arquivo %s enviado. Status: %s\n", filePath, resp.Status)
	} else {
		log.Printf("ERRO API: Falha ao enviar %s. Status: %s\n", filePath, resp.Status)
	}
}
