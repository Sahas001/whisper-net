package main

import (
	"bufio"
	"context"
	"fmt"
	"os"
	"os/signal"
	"strings"
	"sync"
	"syscall"

	"github.com/libp2p/go-libp2p"
	"github.com/libp2p/go-libp2p/core/host"
	"github.com/libp2p/go-libp2p/core/network"
	peerstore "github.com/libp2p/go-libp2p/core/peer"
	"github.com/libp2p/go-libp2p/core/protocol"
	"github.com/libp2p/go-libp2p/p2p/discovery/mdns"
)

type mdnsNotifee struct {
	host host.Host
	name string
}

var (
	ChatProtocol     = protocol.ID("/whisper-net/chat/1.0.0")
	IdentityProtocol = protocol.ID("/whisper-net/identity/1.0.0")
)

var (
	peerNames = make(map[peerstore.ID]string)
	peerMu    sync.RWMutex
)

var connectedPeers sync.Map

func (n *mdnsNotifee) HandlePeerFound(pi peerstore.AddrInfo) {
	if pi.ID == n.host.ID() {
		return
	}

	if _, loaded := connectedPeers.LoadOrStore(pi.ID, struct{}{}); loaded {
		return
	}

	go func() {
		if err := n.host.Connect(context.Background(), pi); err != nil {
			fmt.Println("Failed to connect to discovered peer:", err)
			connectedPeers.Delete(pi.ID)
			return
		}
		sendIdentity(context.Background(), n.host, pi.ID, n.name)
		sendMessage(context.Background(), n.host, pi.ID, "Hello from whisper-net!")
	}()
}

func sendIdentity(ctx context.Context, h host.Host, peerID peerstore.ID, name string) error {
	s, err := h.NewStream(ctx, peerID, IdentityProtocol)
	if err != nil {
		return err
	}
	defer s.Close()

	fmt.Fprintln(s, name)
	return nil
}

func sendMessage(ctx context.Context, h host.Host, peerID peerstore.ID, message string) error {
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

func main() {
	fmt.Println("Enter your name: ")
	reader := bufio.NewReader(os.Stdin)
	name, _ := reader.ReadString('\n')

	node, err := libp2p.New(
		libp2p.ListenAddrStrings("/ip4/127.0.0.1/tcp/0"),
		libp2p.Ping(false),
	)
	if err != nil {
		panic(err)
	}

	service := mdns.NewMdnsService(node, "whisper-net", &mdnsNotifee{
		host: node,
		name: name,
	})

	if err := service.Start(); err != nil {
		panic(err)
	}

	node.SetStreamHandler(IdentityProtocol, func(s network.Stream) {
		defer s.Close()
		reader := bufio.NewReader(s)
		name, err := reader.ReadString('\n')
		if err != nil {
			return
		}
		name = strings.TrimSpace(name)
		peerID := s.Conn().RemotePeer()
		peerMu.Lock()
		peerNames[peerID] = name
		peerMu.Unlock()

		fmt.Printf("Peer %s identified as %q\n", peerID, name)
	})

	node.SetStreamHandler(ChatProtocol, func(s network.Stream) {
		defer s.Close()
		reader := bufio.NewReader(s)
		msg, err := reader.ReadString('\n')
		peerID := s.Conn().RemotePeer()
		peerMu.RLock()
		name, ok := peerNames[peerID]
		peerMu.RUnlock()
		if !ok {
			name = peerID.String()[:8]
		}
		if err != nil {
			fmt.Println("Error reading from stream", err)
			return
		}
		fmt.Printf("Received message from %s: %s", name, msg)
	})

	peerInfo := peerstore.AddrInfo{
		ID:    node.ID(),
		Addrs: node.Addrs(),
	}

	addrs, err := peerstore.AddrInfoToP2pAddrs(&peerInfo)
	if err != nil {
		panic(err)
	}

	fmt.Println("Listening on:", addrs[0])

	ch := make(chan os.Signal, 1)
	signal.Notify(ch, syscall.SIGINT, syscall.SIGTERM)
	<-ch
	fmt.Println()
	fmt.Println("Shutting down the node...")

	if err := node.Close(); err != nil {
		panic(err)
	}
}
