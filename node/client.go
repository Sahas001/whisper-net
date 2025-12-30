package node

import (
	"bufio"
	"context"
	"fmt"
	"os"
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

func isBootstrapPeer(bootstrapPeer []peerstore.AddrInfo, id peerstore.ID) bool {
	for _, p := range bootstrapPeer {
		if p.ID == id {
			return true
		}
	}
	return false
}

func connectAndSendIdentity(ctx context.Context, n host.Host, pi peerstore.AddrInfo, name string) {
	peerMu.RLock()
	if _, sent := identitySent[pi.ID]; sent {
		peerMu.RUnlock()
		return
	}
	peerMu.RUnlock()
	if err := n.Connect(ctx, pi); err != nil {
		fmt.Println("Failed to connect to peer:", err)
		return
	}
	if err := proto.SendIdentity(ctx, n, pi.ID, name); err != nil {
		fmt.Println("Failed to send identity:", err)
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
	node, err := libp2p.New(
		libp2p.ListenAddrStrings("/ip4/0.0.0.0/tcp/0"),
		libp2p.Ping(false),
		libp2p.EnableAutoRelayWithStaticRelays(bootstrapPeers),
		libp2p.EnableHolePunching(),
		libp2p.EnableNATService(),
		libp2p.EnableRelay(),
	)
	if err != nil {
		return err
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

	incoming := make(chan string, 128)

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
			incoming <- fmt.Sprintf("[%s]: %s", name, msg)
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
				go connectAndSendIdentity(ctx, node, p, name)
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

	if err := ui.RunChatUI(ctx, localName, roomID, incoming, sendMessage); err != nil {
		return err
	}

	<-ctx.Done()
	return node.Close()
}
