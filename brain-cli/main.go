package main

import (
	"log"
	"os"

	tea "github.com/charmbracelet/bubbletea"
)

// func main() é o ponto de entrada.
// Ele inicia o Bubble Tea com o 'initialModel()'
func main() {
	// Inicia o programa
	p := tea.NewProgram(initialModel())

	// Roda o programa e checa por erros
	if _, err := p.Run(); err != nil {
		log.Fatalf("Erro ao iniciar o 'brain-cli': %v", err)
		os.Exit(1)
	}
}
