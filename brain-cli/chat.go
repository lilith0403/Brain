package main

import (
	"context"
	"fmt"
	"strings"

	"github.com/charmbracelet/bubbles/spinner"
	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/imroc/req/v3"
)

var (
	inputStyle = lipgloss.NewStyle().Border(lipgloss.RoundedBorder()).BorderForeground(lipgloss.Color("63")).Padding(0, 1)

	aiStyle = lipgloss.NewStyle().Padding(1, 2).Background(lipgloss.Color("240")).Foreground(lipgloss.Color("15")).Margin(1, 0)

	userStyle = lipgloss.NewStyle().Padding(1, 2).Background(lipgloss.Color("63")).Foreground(lipgloss.Color("255")).Margin(1, 0)

	errorStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("9")).Padding(1, 0)
)

type AskRequest struct {
	Query string `json:"query"`
}

type AskResponse struct {
	Status string `json:"status"`
	Answer string `json:"answer"`
}

type model struct {
	textInput textinput.Model
	history   []string
	isLoading bool
	spinner   spinner.Model
	err       error
}

func initialModel() model {
	ti := textinput.New()
	ti.Placeholder = "Pergunte ao Selebro..."
	ti.Focus()
	ti.CharLimit = 250
	ti.Width = 80
	ti.Prompt = "❯ "

	s := spinner.New()
	s.Spinner = spinner.Points
	s.Style = lipgloss.NewStyle().Foreground(lipgloss.Color("63"))

	return model{
		textInput: ti,
		history:   []string{},
		isLoading: false,
		spinner:   s,
		err:       nil,
	}
}

type aiResponseMsg struct {
	answer string
}
type errorMsg struct {
	err error
}

func (m model) Init() tea.Cmd {
	return textinput.Blink
}

func (m model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	var cmd tea.Cmd

	switch msg := msg.(type) {

	case tea.KeyMsg:
		switch msg.Type {
		case tea.KeyCtrlC, tea.KeyEsc:
			return m, tea.Quit

		case tea.KeyEnter:
			if m.isLoading {
				return m, nil
			}

			query := m.textInput.Value()
			if query == "" {
				return m, nil
			}

			m.history = append(m.history, userStyle.Render("Você:\n"+query))
			m.textInput.Reset()
			m.isLoading = true

			return m, tea.Batch(
				m.spinner.Tick,
				func() tea.Msg {
					answer, err := askBrain(query)
					if err != nil {
						return errorMsg{err: err}
					}
					return aiResponseMsg{answer: answer}
				},
			)
		}

	case aiResponseMsg:
		m.isLoading = false
		m.history = append(m.history, aiStyle.Render("Selebro:\n"+msg.answer))
		return m, nil

	case errorMsg:
		m.isLoading = false
		m.err = msg.err
		return m, nil

	case spinner.TickMsg:
		m.spinner, cmd = m.spinner.Update(msg)
		return m, cmd
	}

	m.textInput, cmd = m.textInput.Update(msg)
	return m, cmd
}

func (m model) View() string {
	var s strings.Builder

	s.WriteString("🧠 Bem-vindo ao Selebro (Pressione Ctrl+C para sair)\n\n")

	s.WriteString(strings.Join(m.history, "\n"))
	s.WriteString("\n")

	if m.isLoading {
		s.WriteString(m.spinner.View() + " Pensando...")
	} else if m.err != nil {
		s.WriteString(errorStyle.Render(fmt.Sprintf("Erro: %v", m.err)))
	} else {
		s.WriteString(inputStyle.Render(m.textInput.View()))
	}

	s.WriteString("\n")
	return s.String()
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
