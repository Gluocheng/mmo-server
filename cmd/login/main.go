package main

import (
	"flag"
	"os"

	"github.com/example/mmo-server/internal/loginapp"
)

func main() {
	var (
		profile = flag.String("path", "configs/mmo-cluster.json", "profile json path")
		node    = flag.String("node", "login-1", "node id in profile")
	)
	flag.Parse()
	if *node == "" {
		flag.Usage()
		os.Exit(1)
	}
	loginapp.Run(*profile, *node)
}
