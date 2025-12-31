package ui

// MsgKind categorizes inbound events so the UI can style them.
type MsgKind int

const (
	MsgPeer MsgKind = iota
	MsgNotice
)

// InboundMsg carries text and its kind into the TUI.
type InboundMsg struct {
	Text string
	Kind MsgKind
}
