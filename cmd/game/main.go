package main

import (
	"flag"
	"os"

	"github.com/example/mmo-server/internal/gameapp"
)

func main() {
	var (
		profile = flag.String("path", "configs/mmo-cluster.json", "profile json path")
		node    = flag.String("node", "10001", "game node id (numeric string)")
	)
	flag.Parse()
	if *node == "" {
		flag.Usage()
		os.Exit(1)
	}
	gameapp.Run(*profile, *node)
}
