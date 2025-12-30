package node

import (
	"context"
	"fmt"
	"os"

	"github.com/libp2p/go-libp2p"
	"github.com/libp2p/go-libp2p/core/host"
)

// RunRelayNode starts a libp2p node that acts as a relay (Circuit Relay v2 server)
func RunRelayNode(ctx context.Context, port int, addrFile string) (string, error) {
	node, err := libp2p.New(
		libp2p.ListenAddrStrings(fmt.Sprintf("/ip4/0.0.0.0/tcp/%d", port)),
		libp2p.EnableRelay(),
		libp2p.EnableRelayService(), // circuit v2 relay
	)
	if err != nil {
		return "", err
	}

	if len(node.Addrs()) == 0 {
		return "", fmt.Errorf("no addresses found for relay")
	}

	relayAddr := fmt.Sprintf("%s/p2p/%s", node.Addrs()[0].String(), node.ID().String())

	// Write relay address to file
	if err := os.WriteFile(addrFile, []byte(relayAddr+"\n"), 0o644); err != nil {
		fmt.Println("Failed to write relay address to file:", err)
	} else {
		fmt.Println("Relay address written to", addrFile)
	}

	// Keep node running
	go func() {
		<-ctx.Done()
		node.Close()
	}()

	// Print relay info periodically
	// go func() {
	// 	for {
	// 		select {
	// 		case <-ctx.Done():
	// 			return
	// 		case <-time.After(10 * time.Second):
	fmt.Println("Relay node ID:", node.ID().String())
	for _, addr := range node.Addrs() {
		fmt.Println("Relay listening on:", addr)
	}
	// }
	// }
	// }()

	return relayAddr, nil
}

// NewRelayHost optionally return host if needed
func NewRelayHost(ctx context.Context, port int) (host.Host, error) {
	node, err := libp2p.New(
		libp2p.ListenAddrStrings(fmt.Sprintf("/ip4/0.0.0.0/tcp/%d", port)),
		libp2p.EnableRelay(),
		libp2p.EnableRelayService(),
	)
	if err != nil {
		return nil, err
	}
	return node, nil
}
