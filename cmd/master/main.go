package main

import (
	"flag"
	"os"

	"github.com/example/mmo-server/internal/masterapp"
)

func main() {
	var (
		profile = flag.String("path", "configs/mmo-cluster.json", "profile json path")
		node    = flag.String("node", "master-1", "master node id (must match cluster.nats.master_node_id)")
	)
	flag.Parse()
	if *node == "" {
		flag.Usage()
		os.Exit(1)
	}
	masterapp.Run(*profile, *node)
}
