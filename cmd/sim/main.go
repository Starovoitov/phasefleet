// Command sim runs a fleet in one process.
package main

import (
	"flag"
	"log/slog"
	"os"
)

func main() {
	n := flag.Int("n", 100, "nodes in the fleet")
	mode := flag.String("mode", "desync", "sync, desync, or jitter")
	ticks := flag.Int("ticks", 1000, "simulation steps")
	flag.Parse()

	slog.SetDefault(slog.New(slog.NewTextHandler(os.Stderr, nil)))
	slog.Info("phasefleet sim", "n", *n, "mode", *mode, "ticks", *ticks)
}
