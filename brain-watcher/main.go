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
	"go/pkg/mod", // Bloqueia /home/nisio/go/pkg/mod
	".config/notion-app-enhanced",
	".config/Cursor",
	".config/BraveSoftware",
	".config/discord",
	".config*spotify",
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
		// CORREÇÃO: Checa se o caminho CONTÉM a pasta bloqueada.
		// Adicionamos "/" para evitar falsos positivos (ex: "my-node_modules-project")
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

// Lê o arquivo de configuração e retorna a lista de caminhos
func loadPathsToScan() ([]string, error) {
	configPath := "/home/nisio/.config/brain/scan.paths"
	file, err := os.Open(configPath)
	if err != nil {
		if os.IsNotExist(err) {
			log.Printf("AVISO: Arquivo %s não encontrado.", configPath)
			log.Println("Por favor, crie este arquivo e adicione os caminhos que você quer escanear.")
			log.Println("Exemplo: /home/nisio/Repos")
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
			// Resolve caminhos relativos (ex: ~/Repos)
			if strings.HasPrefix(line, "~/") {
				line = filepath.Join("/home/nisio", line[2:])
			}
			paths = append(paths, line)
		}
	}
	return paths, scanner.Err()
}

func main() {
	watcher, err := fsnotify.NewWatcher()
	if err != nil {
		log.Fatal(err)
	}
	defer watcher.Close()

	// Salva o diretório Home globalmente
	homeDir, err = os.UserHomeDir()
	if err != nil {
		log.Fatal("Erro ao encontrar o diretório home:", err)
	}

	pathsToScan, err := loadPathsToScan()
	if err != nil {
		log.Fatalf("Erro ao ler o arquivo de configuração: %v", err)
	}

	if len(pathsToScan) == 0 {
		log.Println("Nenhum caminho definido em ~/.config/brain/scan.paths. O scan inicial será pulado.")
	} else {
		log.Printf("Iniciando Scan Inicial (Fase 1) em %d caminhos definidos...", len(pathsToScan))
	}

	// Loop por CADA caminho que queremos escanear
	for _, path := range pathsToScan {
		// Garante que o caminho é absoluto
		absPath, err := filepath.Abs(path)
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

			// --- BUG 1 CORRIGIDO: LÓGICA CORRETA ---
			// Se NÃO devemos processar, pule o arquivo/diretório
			if !shouldProcess(walkPath, info.IsDir()) {
				if info.IsDir() {
					log.Printf("IGNORANDO (Scan-Dir): %s\n", walkPath)
					return filepath.SkipDir // Pula este diretório
				}
				// log.Printf("IGNORANDO (Scan-File): %s\n", walkPath)
				return nil // Apenas pula este arquivo
			}
			// --- FIM DA CORREÇÃO ---

			// Se chegou aqui, é porque DEVE ser processado
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

			// --- BUG 2 CORRIGIDO: LÓGICA CORRETA ---
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

// ingestFile (Sem alterações, continua igual)
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

	apiUrl := "http://api:3000/queue/ingest"
	resp, err := http.Post(apiUrl, "application/json", bytes.NewBuffer(jsonPayload))
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
