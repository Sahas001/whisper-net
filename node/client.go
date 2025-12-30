package node

import (
	"bufio"
	"context"
	"fmt"
	"os"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/Sahas001/whisper-net/proto"
	"github.com/Sahas001/whisper-net/ui"
	"github.com/Sahas001/whisper-net/utils"
	"github.com/libp2p/go-libp2p"
	dht "github.com/libp2p/go-libp2p-kad-dht"
	"github.com/libp2p/go-libp2p/core/host"
	"github.com/libp2p/go-libp2p/core/network"
	peerstore "github.com/libp2p/go-libp2p/core/peer"
	routing "github.com/libp2p/go-libp2p/p2p/discovery/routing"
	"github.com/multiformats/go-multiaddr"
)

var (
	peerNames    = make(map[peerstore.ID]string)
	peerMu       sync.RWMutex
	identitySent = make(map[peerstore.ID]struct{})
)

func loadRelayAddr(file string) (multiaddr.Multiaddr, error) {
	data, err := os.ReadFile(file)
	if err != nil {
		return nil, err
	}
	addrStr := strings.TrimSpace(string(data))
	return multiaddr.NewMultiaddr(addrStr)
}

func isBootstrapPeer(bootstrapPeer []peerstore.AddrInfo, id peerstore.ID) bool {
	for _, p := range bootstrapPeer {
		if p.ID == id {
			return true
		}
	}
	return false
}

func sendIdentityWithRetry(ctx context.Context, h host.Host, pid peerstore.ID, name string, notify func(string, ui.MsgKind)) error {
	const maxAttempts = 3
	for attempt := 1; attempt <= maxAttempts; attempt++ {
		attemptCtx, cancel := context.WithTimeout(ctx, 7*time.Second)
		err := proto.SendIdentity(attemptCtx, h, pid, name)
		cancel()
		if err == nil {
			return nil
		}
		if attempt < maxAttempts {
			time.Sleep(time.Duration(attempt) * time.Second)
			continue
		}
		notify(fmt.Sprintf("Failed to send identity to %s after %d attempts: %v", pid, attempt, err), ui.MsgNotice)
		return err
	}
	return nil
}

func connectAndSendIdentity(ctx context.Context, n host.Host, pi peerstore.AddrInfo, name string, notify func(string, ui.MsgKind)) {
	peerMu.RLock()
	if _, sent := identitySent[pi.ID]; sent {
		peerMu.RUnlock()
		return
	}
	peerMu.RUnlock()
	connCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	if err := n.Connect(connCtx, pi); err != nil {
		notify(fmt.Sprintf("Failed to connect to peer %s: %v", pi.ID, err), ui.MsgNotice)
		return
	}
	if err := sendIdentityWithRetry(ctx, n, pi.ID, name, notify); err != nil {
		return
	}
	peerMu.Lock()
	identitySent[pi.ID] = struct{}{}
	peerMu.Unlock()
}

func loadBootstrapPeers(bootstrapFile string) []peerstore.AddrInfo {
	data, err := os.ReadFile(bootstrapFile)
	if err != nil {
		fmt.Println("Failed to read bootstrap address file:", err)
		return nil
	}
	lines := strings.Split(strings.TrimSpace(string(data)), "\n")
	var peers []peerstore.AddrInfo
	for _, line := range lines {
		ai, err := peerstore.AddrInfoFromP2pAddr(multiaddr.StringCast(line))
		if err != nil {
			continue
		}
		peers = append(peers, *ai)
	}
	return peers
}

func RunClientNode(ctx context.Context, bootstrapFile string) error {
	bootstrapPeers := loadBootstrapPeers(bootstrapFile)

	// Load relay address if available
	var relayMultiaddr multiaddr.Multiaddr
	if rma, err := loadRelayAddr("relay.addr"); err == nil {
		relayMultiaddr = rma
		fmt.Println("Using relay:", relayMultiaddr)
	}

	node, err := libp2p.New(
		libp2p.ListenAddrStrings("/ip4/0.0.0.0/tcp/0"),
		libp2p.Ping(false),
		libp2p.EnableHolePunching(),
		libp2p.EnableNATService(),
		libp2p.EnableRelay(),
	)
	if err != nil {
		return err
	}

	// If relay address exists, connect to it
	if relayMultiaddr != nil {
		pi, err := peerstore.AddrInfoFromP2pAddr(relayMultiaddr)
		if err == nil {
			if err := node.Connect(ctx, *pi); err != nil {
				fmt.Println("Failed to connect to relay:", err)
			} else {
				fmt.Println("Connected to relay:", pi.ID)
			}
		}
	}

	kademliaDHT, err := dht.New(ctx, node, dht.Mode(dht.ModeAutoServer), dht.BootstrapPeers(bootstrapPeers...))
	if err != nil {
		return err
	}
	if err := kademliaDHT.Bootstrap(ctx); err != nil {
		return err
	}

	routingDiscovery := routing.NewRoutingDiscovery(kademliaDHT)

	// Ask for client name
	fmt.Print("Enter your name: ")
	reader := bufio.NewReader(os.Stdin)
	name, _ := reader.ReadString('\n')
	name = strings.TrimSpace(name)
	localName := name
	if localName == "" {
		localName = node.ID().String()[:8]
	}
	fmt.Printf("Your name is set to %q\n", localName)

	fmt.Println("Enter room ID (Blank to generate): ")
	roomID, _ := reader.ReadString('\n')
	roomID = strings.TrimSpace(roomID)
	if roomID == "" {
		roomID = utils.GenerateRoomID()
		fmt.Println("Generated room ID:", roomID)
	} else {
		fmt.Println("Using room ID:", roomID)
	}

	rendezvous := fmt.Sprintf("%s-%s", proto.Namespace, roomID)

	fmt.Println("Joining room:", rendezvous)

	incoming := make(chan ui.InboundMsg, 128)

	notify := func(text string, kind ui.MsgKind) {
		select {
		case incoming <- ui.InboundMsg{Text: text, Kind: kind}:
		default:
		}
	}

	for _, p := range bootstrapPeers {
		if err := node.Connect(ctx, p); err != nil {
			fmt.Println("Failed to connect to bootstrap peer:", err)
		} else {
			fmt.Println("Connected to bootstrap peer:", p.ID)
		}
	}

	node.SetStreamHandler(proto.IdentityProtocol, func(s network.Stream) {
		defer s.Close()
		reader := bufio.NewReader(s)
		remoteName, err := reader.ReadString('\n')
		if err != nil {
			return
		}
		remoteName = strings.TrimSpace(remoteName)
		peerID := s.Conn().RemotePeer()
		peerMu.Lock()
		_, existed := peerNames[peerID]
		peerNames[peerID] = remoteName
		_, sent := identitySent[peerID]
		peerMu.Unlock()

		if !existed {
			notify(fmt.Sprintf("%s joined the room", remoteName), ui.MsgNotice)
		}

		if !sent {
			if err := proto.SendIdentity(ctx, node, peerID, localName); err == nil {
				peerMu.Lock()
				identitySent[peerID] = struct{}{}
				peerMu.Unlock()
			} else {
				fmt.Println("Failed to reply with identity:", err)
			}
		}
	})

	node.SetStreamHandler(proto.ChatProtocol, func(s network.Stream) {
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
		msg = strings.TrimSpace(msg)
		if msg != "" {
			incoming <- ui.InboundMsg{Text: fmt.Sprintf("%s: %s", name, msg), Kind: ui.MsgPeer}
		}
	})

	// Advertise peers
	go func() {
		for {
			peers := kademliaDHT.RoutingTable().ListPeers()
			if len(peers) == 0 {
				time.Sleep(1 * time.Second)
				continue
			}
			ttl, err := routingDiscovery.Advertise(ctx, proto.Namespace)
			if err != nil {
				fmt.Println("Error advertising:", err)
			} else {
				fmt.Println("Advertising with TTL:", ttl)
			}
			time.Sleep(ttl)
		}
	}()

	// Discover peers
	go func() {
		for {
			peers, err := routingDiscovery.FindPeers(ctx, proto.Namespace)
			if err != nil {
				time.Sleep(2 * time.Second)
				continue
			}

			for p := range peers {
				if p.ID == node.ID() {
					continue
				}
				if isBootstrapPeer(bootstrapPeers, p.ID) {
					continue
				}
				pi := p
				if len(pi.Addrs) == 0 {
					lookupCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
					info, err := kademliaDHT.FindPeer(lookupCtx, p.ID)
					cancel()
					if err != nil {
						continue
					}
					pi = info
				}
				go connectAndSendIdentity(ctx, node, p, name, notify)
			}
			time.Sleep(5 * time.Second)
		}
	}()
	sendMessage := func(text string) error {
		peerMu.RLock()
		peers := make([]peerstore.ID, 0, len(peerNames))
		for id := range peerNames {
			peers = append(peers, id)
		}
		peerMu.RUnlock()
		if len(peers) == 0 {
			return fmt.Errorf("no peers in room yet")
		}
		var lastErr error
		for _, pid := range peers {
			if err := proto.SendMessage(ctx, node, pid, fmt.Sprintf("%s: %s", localName, text)); err != nil {
				lastErr = err
			}
		}
		return lastErr
	}
	peerSummary := func() (int, []string) {
		peerMu.RLock()
		names := make([]string, 0, len(peerNames))
		for _, n := range peerNames {
			names = append(names, n)
		}
		peerMu.RUnlock()
		sort.Strings(names)
		return len(names), names
	}

	if err := ui.RunChatUI(ctx, localName, roomID, incoming, sendMessage, peerSummary); err != nil {
		return err
	}

	<-ctx.Done()
	return node.Close()
}
