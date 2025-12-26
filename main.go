package main

import (
	"bufio"
	"context"
	"fmt"
	"os"
	"os/signal"
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
}

const ChatProtocol = protocol.ID("/whisper/1.0.0")

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
		sendMessage(context.Background(), n.host, pi.ID, "Hello from "+n.host.ID().String())
	}()
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
	node, err := libp2p.New(
		libp2p.ListenAddrStrings("/ip4/127.0.0.1/tcp/0"),
		libp2p.Ping(false),
	)
	if err != nil {
		panic(err)
	}

	service := mdns.NewMdnsService(node, "whisper-net", &mdnsNotifee{host: node})

	if err := service.Start(); err != nil {
		panic(err)
	}

	node.SetStreamHandler(ChatProtocol, func(s network.Stream) {
		defer s.Close()
		reader := bufio.NewReader(s)
		msg, err := reader.ReadString('\n')
		if err != nil {
			fmt.Println("Error reading from stream", err)
			return
		}
		fmt.Printf("Received message from %s: %s", s.Conn().RemotePeer(), msg)
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
