package main

import (
	"context"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/Ltre/FmlySys/internal/config"
	"github.com/Ltre/FmlySys/internal/httpserver"
	"github.com/Ltre/FmlySys/internal/partition"
	"github.com/Ltre/FmlySys/internal/store"
)

func main() {
	cfg := config.Load()
	ctx := context.Background()
	pm, err := partition.Open(ctx, cfg.DataDir)
	if err != nil {
		log.Fatal(err)
	}
	defer pm.Close()
	st := store.New(pm.ActiveDB)
	actorID, err := st.EnsureDevMember(ctx, cfg.DevMember)
	if err != nil {
		log.Fatal(err)
	}
	app, err := httpserver.New(pm, st, actorID)
	if err != nil {
		log.Fatal(err)
	}
	srv := &http.Server{Addr: cfg.Addr, Handler: app.Handler(), ReadHeaderTimeout: 5 * time.Second}
	go func() {
		log.Printf("FmlySys listening on %s, partition=%s", cfg.Addr, pm.ActiveID)
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Fatal(err)
		}
	}()
	ch := make(chan os.Signal, 1)
	signal.Notify(ch, syscall.SIGTERM, syscall.SIGINT)
	<-ch
	shutdown, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	_ = srv.Shutdown(shutdown)
}
