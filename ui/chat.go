// Package ui creates chat UI using Bubble Tea framework
package ui

import (
	"context"
	"fmt"
	"strings"

	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

type (
	incomingMsg string
	sendErrMsg  struct{ err error }
)

type chatLine struct {
	text  string
	style lipgloss.Style
}

type chatModel struct {
	localName string
	room      string
	messages  []chatLine
	incoming  <-chan string
	send      func(string) error
	input     textinput.Model
	errText   string
	styles    chatStyles
}

type chatStyles struct {
	header lipgloss.Style
	box    lipgloss.Style
	self   lipgloss.Style
	peer   lipgloss.Style
	err    lipgloss.Style
}

func newChatModel(localName, room string, incoming <-chan string, send func(string) error) chatModel {
	ti := textinput.New()
	ti.Placeholder = "Type message and press Enter"
	ti.Focus()
	ti.Prompt = "> "

	styles := chatStyles{
		header: lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("13")),
		box:    lipgloss.NewStyle().Border(lipgloss.RoundedBorder()).Padding(1, 1),
		self:   lipgloss.NewStyle().Foreground(lipgloss.Color("10")),
		peer:   lipgloss.NewStyle().Foreground(lipgloss.Color("12")),
		err:    lipgloss.NewStyle().Foreground(lipgloss.Color("1")),
	}
	return chatModel{
		localName: localName,
		room:      room,
		messages:  []chatLine{{text: "Welcome to the room. Start typing to chat.", style: styles.peer}},
		incoming:  incoming,
		send:      send,
		input:     ti,
		styles:    styles,
	}
}

func waitIncoming(ch <-chan string) tea.Cmd {
	return func() tea.Msg {
		msg, ok := <-ch
		if !ok {
			return nil
		}
		return incomingMsg(msg)
	}
}

func (m chatModel) Init() tea.Cmd {
	return tea.Batch(textinput.Blink, waitIncoming(m.incoming))
}

func (m chatModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case incomingMsg:
		if msg != "" {
			m.appendMessage(string(msg), m.styles.peer)
		}
		return m, waitIncoming(m.incoming)
	case sendErrMsg:
		m.errText = msg.err.Error()
		m.appendMessage("Error: "+m.errText, m.styles.err)
		return m, nil
	case tea.KeyMsg:
		if m.errText != "" {
			m.errText = ""
		}
		if msg.Type == tea.KeyEnter {
			text := strings.TrimSpace(m.input.Value())
			if text == "" {
				return m, nil
			}
			m.appendMessage(fmt.Sprintf("%s (you): %s", m.localName, text), m.styles.self)
			m.input.SetValue("")
			return m, tea.Batch(waitIncoming(m.incoming), sendCmd(m.send, text))
		}
	}

	var cmd tea.Cmd
	m.input, cmd = m.input.Update(msg)
	return m, cmd
}

func (m chatModel) View() string {
	header := m.styles.header.Render(fmt.Sprintf("Room: %s | You: %s", m.room, m.localName))
	lines := make([]string, len(m.messages))
	for i, line := range m.messages {
		lines[i] = line.style.Render(line.text)
	}
	body := strings.Join(lines, "\n")
	prompt := m.input.View()

	if m.errText != "" {
		prompt += "\n" + m.styles.err.Render(m.errText)
	}

	return m.styles.box.Render(header + "\n\n" + body + "\n\n" + prompt)
}

func (m *chatModel) appendMessage(msg string, style lipgloss.Style) {
	m.messages = append(m.messages, chatLine{text: msg, style: style})
	if len(m.messages) > 200 {
		m.messages = m.messages[len(m.messages)-200:]
	}
}

func sendCmd(send func(string) error, text string) tea.Cmd {
	return func() tea.Msg {
		if err := send(text); err != nil {
			return sendErrMsg{err: err}
		}
		return nil
	}
}

func RunChatUI(ctx context.Context, localName, room string, incoming <-chan string, send func(string) error) error {
	model := newChatModel(localName, room, incoming, send)
	p := tea.NewProgram(model)
	go func() {
		<-ctx.Done()
		p.Quit()
	}()
	_, err := p.Run()
	return err
}
