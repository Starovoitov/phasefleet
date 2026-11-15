// Command node is one fleet member. It will gossip phases over UDP.
package main

import (
	"flag"
	"log/slog"
	"os"
)

func main() {
	listen := flag.String("listen", ":9000", "UDP listen address")
	flag.Parse()

	slog.SetDefault(slog.New(slog.NewTextHandler(os.Stderr, nil)))
	slog.Info("phasefleet node", "listen", *listen)
}
