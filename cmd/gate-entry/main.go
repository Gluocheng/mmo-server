package main

import (
	"flag"
	"log"
	"os"

	"github.com/example/mmo-server/internal/gateentry"
)

func main() {
	profile := flag.String("path", "configs/mmo-cluster.json", "profile json path")
	flag.Parse()
	cfg, err := gateentry.LoadConfig(*profile)
	if err != nil {
		log.Fatal(err)
	}
	ln, err := gateentry.Listen(cfg.Listen)
	if err != nil {
		log.Fatal(err)
	}
	log.Printf("gate-entry listen %s gates=%d max=%d", cfg.Listen, len(cfg.Gates), cfg.MaxConnectionsPerGate)
	if err := gateentry.Serve(ln, gateentry.NewBalancer(cfg.Gates)); err != nil {
		log.Println(err)
		os.Exit(1)
	}
}
