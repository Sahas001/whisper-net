// Package node
package node

import (
	"context"
	"fmt"
	"os"
	"time"

	"github.com/Sahas001/whisper-net/proto"
	"github.com/libp2p/go-libp2p"
	dht "github.com/libp2p/go-libp2p-kad-dht"
	routing "github.com/libp2p/go-libp2p/p2p/discovery/routing"
)

func RunBootstrapPeer(ctx context.Context, listenHost string, port int) (string, error) {
	addrStr := fmt.Sprintf("/ip4/%s/tcp/%d", listenHost, port)

	host, err := libp2p.New(libp2p.ListenAddrStrings(addrStr))
	if err != nil {
		return "", err
	}

	kademliaDHT, err := dht.New(ctx, host, dht.Mode(dht.ModeServer))
	if err != nil {
		return "", err
	}
	kademliaDHT.Bootstrap(ctx)

	routingDiscovery := routing.NewRoutingDiscovery(kademliaDHT)

	// Advertise periodically so clients can discover
	go func() {
		for {
			ttl, err := routingDiscovery.Advertise(ctx, proto.Namespace)
			if err == nil {
				fmt.Println("Bootstrap advertised, TTL:", ttl)
			}
			time.Sleep(5 * time.Second)
		}
	}()

	// Write address to file for clients
	fullAddr := fmt.Sprintf("%s/p2p/%s", addrStr, host.ID().String())
	f, err := os.Create("bootstrap_peer.addr")
	if err != nil {
		return "", err
	}
	defer f.Close()
	_, _ = f.WriteString(fullAddr + "\n")

	return fullAddr, nil
}
