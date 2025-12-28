// Package proto
package proto

import (
	"bufio"
	"context"
	"fmt"

	"github.com/libp2p/go-libp2p/core/host"
	peerstore "github.com/libp2p/go-libp2p/core/peer"
	"github.com/libp2p/go-libp2p/core/protocol"
)

var (
	ChatProtocol     = protocol.ID("/whisper-net/chat/1.0.0")
	IdentityProtocol = protocol.ID("/whisper-net/identity/1.0.0")
	Namespace        = "whisper-net"
)

func SendIdentity(ctx context.Context, h host.Host, peerID peerstore.ID, name string) error {
	s, err := h.NewStream(ctx, peerID, IdentityProtocol)
	if err != nil {
		return err
	}
	defer s.Close()

	fmt.Fprintln(s, name)
	return nil
}

func SendMessage(ctx context.Context, h host.Host, peerID peerstore.ID, message string) error {
	s, err := h.NewStream(ctx, peerID, ChatProtocol)
	if err != nil {
		return err
	}
	defer s.Close()

	writer := bufio.NewWriter(s)
	_, err = writer.WriteString(message + "\n")
	if err != nil {
		return err
	}
	return writer.Flush()
}
