// Package ui implements a terminal-based chat user interface using the Bubble Tea framework.
package ui

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/charmbracelet/bubbles/textinput"
	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

type (
	incomingMsg InboundMsg
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
	incoming  <-chan InboundMsg
	send      func(string) error
	peers     func() (int, []string)
	input     textinput.Model
	errText   string
	status    string
	styles    chatStyles
	vp        viewport.Model
	ready     bool
	notice    string
	noticeAt  time.Time
}

type chatStyles struct {
	header lipgloss.Style
	box    lipgloss.Style
	self   lipgloss.Style
	peer   lipgloss.Style
	err    lipgloss.Style
	status lipgloss.Style
	help   lipgloss.Style
	info   lipgloss.Style
	notice lipgloss.Style
}

func newChatModel(localName, room string, incoming <-chan InboundMsg, send func(string) error, peers func() (int, []string)) chatModel {
	ti := textinput.New()
	ti.Placeholder = "Type a message"
	ti.Focus()
	ti.Prompt = "> "
	ti.Width = 50

	styles := chatStyles{
		header: lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("13")),
		box:    lipgloss.NewStyle().Border(lipgloss.RoundedBorder()).Padding(1, 1).BorderForeground(lipgloss.Color("241")),
		self:   lipgloss.NewStyle().Foreground(lipgloss.Color("10")),
		peer:   lipgloss.NewStyle().Foreground(lipgloss.Color("12")),
		err:    lipgloss.NewStyle().Foreground(lipgloss.Color("1")),
		status: lipgloss.NewStyle().Foreground(lipgloss.Color("11")),
		help:   lipgloss.NewStyle().Foreground(lipgloss.Color("245")),
		info:   lipgloss.NewStyle().Foreground(lipgloss.Color("14")),
		notice: lipgloss.NewStyle().Foreground(lipgloss.Color("51")),
	}
	return chatModel{
		localName: localName,
		room:      room,
		messages:  []chatLine{{text: "Welcome to the room. Start typing to chat.", style: styles.peer}},
		incoming:  incoming,
		send:      send,
		peers:     peers,
		input:     ti,
		styles:    styles,
	}
}

func waitIncoming(ch <-chan InboundMsg) tea.Cmd {
	return func() tea.Msg {
		msg, ok := <-ch
		if !ok {
			return incomingMsg{}
		}
		return incomingMsg(msg)
	}
}

func (m chatModel) Init() tea.Cmd {
	return tea.Batch(textinput.Blink, waitIncoming(m.incoming))
}

func (m chatModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.ready = true
		m.resizeViewport(msg)
		return m, nil
	case incomingMsg:
		if msg.Text == "" {
			m.status = "Disconnected from incoming feed"
			return m, nil
		}
		switch msg.Kind {
		case MsgNotice:
			m.notice = msg.Text
			m.noticeAt = time.Now()
			m.appendMessage(msg.Text, m.styles.notice)
		default:
			m.appendMessage(msg.Text, m.styles.peer)
		}
		return m, waitIncoming(m.incoming)
	case sendErrMsg:
		m.errText = msg.err.Error()
		m.status = m.errText
		return m, nil
	case tea.KeyMsg:
		if m.errText != "" {
			m.errText = ""
		}
		if m.status != "" {
			m.status = ""
		}
		if msg.Type == tea.KeyCtrlC || msg.Type == tea.KeyEsc {
			return m, tea.Quit
		}
		if msg.Type == tea.KeyEnter {
			text := strings.TrimSpace(m.input.Value())
			if text == "" {
				return m, nil
			}
			m.appendMessage(fmt.Sprintf("%s (you): %s", m.localName, text), m.styles.self)
			m.input.SetValue("")
			m.syncViewport()
			return m, tea.Batch(waitIncoming(m.incoming), sendCmd(m.send, text))
		}
	}

	var cmd tea.Cmd
	m.input, cmd = m.input.Update(msg)
	return m, cmd
}

func (m chatModel) View() string {
	header := m.styles.header.Render(fmt.Sprintf("Room: %s | You: %s", m.room, m.localName))
	content := m.vp.View()
	prompt := m.input.View()

	noticeView := ""
	if m.notice != "" && time.Since(m.noticeAt) < 8*time.Second {
		noticeBox := m.styles.box.BorderForeground(lipgloss.Color("51"))
		noticeView = noticeBox.Render(m.styles.notice.Render(m.notice))
	}

	boxWidth := max(m.vp.Width, 30)

	statusLine := ""
	if m.status != "" {
		statusLine = m.styles.status.Render(m.status)
	} else {
		count, names := m.peers()
		summary := fmt.Sprintf("You: %s • Room: %s • Peers: %d", m.localName, m.room, count)
		if len(names) > 0 {
			summary += " [" + strings.Join(names, ", ") + "]"
		}
		statusLine = m.styles.info.Render(summary)
	}

	msgBox := m.styles.box.Width(boxWidth).Render(content)
	statusBox := m.styles.box.Width(boxWidth).Render(statusLine)
	help := m.styles.help.Render("Enter to send • Esc/Ctrl+C to quit")

	parts := []string{header}
	if noticeView != "" {
		parts = append(parts, noticeView)
	}
	parts = append(parts, msgBox, prompt, statusBox, help)

	return lipgloss.JoinVertical(lipgloss.Left, parts...)
}

func (m *chatModel) appendMessage(msg string, style lipgloss.Style) {
	m.messages = append(m.messages, chatLine{text: msg, style: style})
	if len(m.messages) > 200 {
		m.messages = m.messages[len(m.messages)-200:]
	}
	m.syncViewport()
}

func (m *chatModel) syncViewport() {
	lines := make([]string, len(m.messages))
	for i, line := range m.messages {
		lines[i] = line.style.Render(line.text)
	}
	m.vp.SetContent(strings.Join(lines, "\n"))
	m.vp.GotoBottom()
}

func (m *chatModel) resizeViewport(msg tea.WindowSizeMsg) {
	if msg.Width <= 0 || msg.Height <= 0 {
		return
	}
	borderPad := 4 // approximate padding+border from box style
	headerH := 1
	promptH := 1
	statusH := 2 // status box plus spacing
	helpH := 1
	extra := 2 // spacing between sections
	availH := max(msg.Height-(headerH+promptH+statusH+helpH+extra), 3)
	availW := max(msg.Width-borderPad, 30)
	m.vp.Width = availW
	m.vp.Height = availH
	m.syncViewport()
}

func sendCmd(send func(string) error, text string) tea.Cmd {
	return func() tea.Msg {
		if err := send(text); err != nil {
			return sendErrMsg{err: err}
		}
		return nil
	}
}

func RunChatUI(ctx context.Context, localName, room string, incoming <-chan InboundMsg, send func(string) error, peers func() (int, []string)) error {
	model := newChatModel(localName, room, incoming, send, peers)
	p := tea.NewProgram(model, tea.WithAltScreen())
	go func() {
		<-ctx.Done()
		p.Quit()
	}()
	_, err := p.Run()
	return err
}
