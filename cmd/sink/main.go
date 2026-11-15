// Command sink records task firings and reports peak concurrency.
package main

import (
	"flag"
	"log/slog"
	"os"
)

func main() {
	listen := flag.String("listen", ":8080", "HTTP listen address")
	flag.Parse()

	slog.SetDefault(slog.New(slog.NewTextHandler(os.Stderr, nil)))
	slog.Info("phasefleet sink", "listen", *listen)
}
