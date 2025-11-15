package main

import (
	"context"
	"fmt"
	"regexp"
	"strings"

	"github.com/charmbracelet/bubbles/spinner"
	"github.com/charmbracelet/bubbles/textinput"
	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/glamour"
	"github.com/charmbracelet/lipgloss"
	"github.com/imroc/req/v3"
)

var (
	// Estilos para o header
	headerStyle = lipgloss.NewStyle().
			Bold(true).
			Foreground(lipgloss.Color("63")).
			Padding(0, 1).
			MarginBottom(1)

	// Estilos para mensagens do usuário
	userLabelStyle = lipgloss.NewStyle().
			Bold(true).
			Foreground(lipgloss.Color("39")).
			MarginRight(1)

	userBubbleStyle = lipgloss.NewStyle().
			Padding(1, 2).
			Background(lipgloss.Color("39")).
			Foreground(lipgloss.Color("255")).
			Margin(1, 0).
			Border(lipgloss.RoundedBorder()).
			BorderForeground(lipgloss.Color("39"))

	// Estilos para mensagens do AI
	aiLabelStyle = lipgloss.NewStyle().
			Bold(true).
			Foreground(lipgloss.Color("135")).
			MarginRight(1)

	aiBubbleStyle = lipgloss.NewStyle().
			Padding(1, 2).
			Margin(1, 0).
			Border(lipgloss.RoundedBorder()).
			BorderForeground(lipgloss.Color("135")).
			Width(0) // Será ajustado dinamicamente

	// Estilo para input
	inputStyle = lipgloss.NewStyle().
			Border(lipgloss.RoundedBorder()).
			BorderForeground(lipgloss.Color("63")).
			Padding(0, 1).
			MarginTop(1).
			Background(lipgloss.NoColor{})

	// Estilo para erros
	errorStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("9")).
			Padding(1, 0).
			Bold(true)

	// Estilo para loading
	loadingStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("63")).
			MarginTop(1)
)

type AskRequest struct {
	Query string `json:"query"`
}

type AskResponse struct {
	Status string `json:"status"`
	Answer string `json:"answer"`
}

type message struct {
	role    string // "user" ou "ai"
	content string
}

type model struct {
	textInput textinput.Model
	spinner   spinner.Model
	viewport  viewport.Model
	messages  []message
	isLoading bool
	err       error
	width     int
	height    int
}

func initialModel() model {
	ti := textinput.New()
	ti.Placeholder = "Pergunte ao Selebro..."
	ti.Focus()
	ti.CharLimit = 500
	ti.Prompt = "❯ "
	ti.PromptStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("63"))
	ti.Cursor.Style = lipgloss.NewStyle().Foreground(lipgloss.Color("63"))

	s := spinner.New()
	s.Spinner = spinner.Points
	s.Style = lipgloss.NewStyle().Foreground(lipgloss.Color("63"))

	vp := viewport.New(80, 20)

	return model{
		textInput: ti,
		spinner:   s,
		viewport:  vp,
		messages:  []message{},
		isLoading: false,
		err:       nil,
		width:     80,
		height:    24,
	}
}

type aiResponseMsg struct {
	answer string
}

type errorMsg struct {
	err error
}

type windowSizeMsg struct {
	width  int
	height int
}

func (m model) Init() tea.Cmd {
	return tea.Batch(
		textinput.Blink,
		tea.EnterAltScreen,
	)
}

func (m model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	var cmd tea.Cmd
	var cmds []tea.Cmd

	switch msg := msg.(type) {

	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height

		// Ajusta o viewport (deixa espaço para header e input)
		headerHeight := 2
		inputHeight := 3
		m.viewport.Width = msg.Width
		if msg.Height > headerHeight+inputHeight {
			m.viewport.Height = msg.Height - headerHeight - inputHeight
		} else {
			m.viewport.Height = 10 // mínimo
		}

		// Ajusta o text input (deixa margem para bordas)
		if msg.Width > 8 {
			m.textInput.Width = msg.Width - 6
		} else {
			m.textInput.Width = 20
		}

		// Re-renderiza o conteúdo
		m.viewport.SetContent(m.renderHistory())
		m.viewport.GotoBottom()
		return m, nil

	case tea.KeyMsg:
		switch msg.Type {
		case tea.KeyCtrlC, tea.KeyEsc:
			return m, tea.ExitAltScreen

		case tea.KeyEnter:
			if m.isLoading {
				return m, nil
			}

			query := m.textInput.Value()
			if query == "" {
				return m, nil
			}

			m.messages = append(m.messages, message{
				role:    "user",
				content: query,
			})
			m.textInput.Reset()
			m.isLoading = true
			m.viewport.SetContent(m.renderHistory())
			m.viewport.GotoBottom()

			cmds = append(cmds,
				m.spinner.Tick,
				func() tea.Msg {
					answer, err := askBrain(query)
					if err != nil {
						return errorMsg{err: err}
					}
					return aiResponseMsg{answer: answer}
				},
			)
			return m, tea.Batch(cmds...)

		case tea.KeyUp, tea.KeyDown:
			// Permite scroll no viewport quando não está digitando
			if !m.textInput.Focused() {
				m.viewport, cmd = m.viewport.Update(msg)
				return m, cmd
			}
		}

	case aiResponseMsg:
		m.isLoading = false
		m.messages = append(m.messages, message{
			role:    "ai",
			content: msg.answer,
		})
		m.viewport.SetContent(m.renderHistory())
		m.viewport.GotoBottom()
		return m, nil

	case errorMsg:
		m.isLoading = false
		m.err = msg.err
		m.viewport.SetContent(m.renderHistory())
		return m, nil

	case spinner.TickMsg:
		m.spinner, cmd = m.spinner.Update(msg)
		return m, cmd
	}

	m.textInput, cmd = m.textInput.Update(msg)
	m.viewport, _ = m.viewport.Update(msg)
	return m, cmd
}

func (m model) renderHistory() string {
	if len(m.messages) == 0 {
		return ""
	}

	var sections []string

	for _, msg := range m.messages {
		var rendered string

		// Calcula largura disponível (deixa margem para bordas e padding)
		availableWidth := m.width - 8
		if availableWidth < 20 {
			availableWidth = 20
		}

		if msg.role == "user" {
			// Mensagem do usuário - simples, sem markdown
			content := lipgloss.NewStyle().
				Width(availableWidth).
				Render(msg.content)

			bubbleWidth := availableWidth + 4
			rendered = userLabelStyle.Render("Você:") + "\n" +
				userBubbleStyle.Width(bubbleWidth).Render(content)
		} else {
			// Mensagem do AI - com markdown
			content, err := renderMarkdown(msg.content, availableWidth-4)
			if err != nil {
				// Fallback se markdown falhar
				content = lipgloss.NewStyle().
					Width(availableWidth - 4).
					Render(msg.content)
			}

			bubbleWidth := availableWidth + 4
			// Não aplica background para não interferir com as cores do markdown
			rendered = aiLabelStyle.Render("Selebro:") + "\n" +
				aiBubbleStyle.Width(bubbleWidth).Render(content)
		}

		sections = append(sections, rendered)
	}

	// Adiciona loading ou erro
	if m.isLoading {
		loadingText := loadingStyle.Render(m.spinner.View() + " Pensando...")
		sections = append(sections, loadingText)
	} else if m.err != nil {
		errorText := errorStyle.Render(fmt.Sprintf("❌ Erro: %v", m.err))
		sections = append(sections, errorText)
	}

	return strings.Join(sections, "\n\n")
}

func renderMarkdown(text string, width int) (string, error) {
	// Configura o glamour para renderizar markdown
	// Usa estilo "notty" que é mais limpo e não aplica backgrounds
	rendered, err := glamour.Render(text, "notty")
	if err != nil {
		// Fallback para auto se notty falhar
		rendered, err = glamour.Render(text, "auto")
		if err != nil {
			return "", err
		}
	}

	// Retorna o markdown renderizado sem aplicar estilos adicionais
	// O glamour já faz todo o trabalho de formatação e word wrap
	return rendered, nil
}

func (m model) View() string {
	// Header
	header := headerStyle.Width(m.width).Render("🧠 Selebro - Seu Assistente Inteligente (Ctrl+C para sair)")

	// Viewport com histórico
	m.viewport.SetContent(m.renderHistory())
	historyView := m.viewport.View()

	// Input - ajusta largura baseado no tamanho disponível
	inputWidth := m.width - 4
	if inputWidth < 20 {
		inputWidth = 20
	}
	// Renderiza o input e remove códigos OSC (Operating System Command) que podem aparecer
	inputText := m.textInput.View()
	// Remove códigos OSC como ]11;rgb:... que alguns terminais enviam
	oscPattern := regexp.MustCompile(`\]\d+;[^\x07]*\x07?`)
	inputText = oscPattern.ReplaceAllString(inputText, "")
	inputView := inputStyle.Width(inputWidth).Render(inputText)

	// Combina tudo
	return lipgloss.JoinVertical(
		lipgloss.Left,
		header,
		historyView,
		inputView,
	)
}

func askBrain(q string) (string, error) {
	apiUrl := "http://localhost:3000/ai/ask"
	client := req.C()
	payload := AskRequest{Query: q}
	var responseData AskResponse

	resp, err := client.R().
		SetContext(context.Background()).
		SetBody(&payload).
		SetHeader("Content-Type", "application/json").
		SetSuccessResult(&responseData).
		Post(apiUrl)

	if err != nil {
		return "", fmt.Errorf("falha na requisição: %w", err)
	}
	if !resp.IsSuccessState() {
		return "", fmt.Errorf("erro da API (%s): %s", resp.Status, resp.String())
	}

	return responseData.Answer, nil
}
