package main

import (
	"context"
	"fmt"
	"log"
	"os"
	"os/signal"
	"syscall"

	"github.com/Sahas001/whisper-net/node"
)

func main() {
	if len(os.Args) < 2 {
		log.Fatal("Usage: app [bootstrap|client]")
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	ch := make(chan os.Signal, 1)
	signal.Notify(ch, syscall.SIGINT, syscall.SIGTERM)
	go func() {
		<-ch
		cancel()
	}()
	role := os.Args[1]
	switch role {
	case "bootstrap":
		addr, err := node.RunBootstrapPeer(ctx, "127.0.0.1", 4001)
		if err != nil {
			log.Fatal(err)
		}
		fmt.Println("Bootstrap peer running at:", addr)

		<-ctx.Done()

	case "client":
		if len(os.Args) < 3 {
			fmt.Println("Redirecting to official libp2p bootstrap node...")
			if err := node.RunClientNode(ctx, "libp2p_bootstrap_peer.addr"); err != nil {
				log.Fatal(err)
			}
		} else {
			bootstrapAddr := os.Args[2]
			if err := node.RunClientNode(ctx, bootstrapAddr); err != nil {
				log.Fatal(err)
			}

		}
	// case "relay":
	// 	addrFile := "relay.addr"
	// 	addr, err := node.RunRelayNode(ctx, 4002, addrFile)
	// 	if err != nil {
	// 		log.Fatal(err)
	// 	}
	// 	fmt.Println("Relay node running at:", addr)
	// 	<-ctx.Done()
	default:
		log.Fatal("Unknown role:", role)
	}
}
