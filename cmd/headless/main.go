// headless is a diagnostic entry point that starts the REST server without
// the Wails GUI. Useful for smoke-testing the pipeline via curl.
package main

import (
	"context"
	"flag"
	"log"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"

	"github.com/geekychris/mac_transformer/internal/core"
	"github.com/geekychris/mac_transformer/internal/httpapi"
	"github.com/geekychris/mac_transformer/internal/plugin"
)

func main() {
	pluginsFlag := flag.String("plugins", "", "path to bundled plugins dir (default: cwd/plugins)")
	flag.Parse()

	bundled := *pluginsFlag
	if bundled == "" {
		wd, _ := os.Getwd()
		bundled = filepath.Join(wd, "plugins")
	}

	store, _ := core.LoadStore()
	cfg := store.Get()
	registry, errs := plugin.Load(context.Background(), []string{bundled, cfg.UserPluginDir})
	for _, e := range errs {
		log.Printf("plugin load: %v", e)
	}
	log.Printf("loaded %d plugins (%d available)", len(registry.All()), len(registry.Available()))
	for _, lp := range registry.All() {
		status := "OK"
		if !lp.Available {
			status = "SKIP: " + lp.Reason
		}
		log.Printf("  %-32s %s", lp.Manifest.Name, status)
	}

	dispatcher := &core.Dispatcher{Registry: registry, Config: store}
	queue := core.NewQueue(dispatcher, nil)
	s := &httpapi.Server{Queue: queue, Registry: registry, Store: store}
	if err := s.Start(); err != nil {
		log.Fatal(err)
	}
	log.Printf("REST listening on 127.0.0.1:%d", cfg.Port)

	sig := make(chan os.Signal, 1)
	signal.Notify(sig, syscall.SIGINT, syscall.SIGTERM)
	<-sig
	log.Println("shutting down")
}
