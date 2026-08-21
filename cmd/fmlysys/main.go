package main

import (
	"context"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/Ltre/FmlySys/internal/adminauth"
	"github.com/Ltre/FmlySys/internal/config"
	"github.com/Ltre/FmlySys/internal/httpserver"
	"github.com/Ltre/FmlySys/internal/partition"
	"github.com/Ltre/FmlySys/internal/store"
)

func main() {
	cfg, err := config.Load()
	if err != nil {
		log.Fatal(err)
	}
	ctx := context.Background()
	pm, err := partition.Open(ctx, cfg.DataDir)
	if err != nil {
		log.Fatal(err)
	}
	defer pm.Close()

	st := store.New(pm.ActiveDB)
	devActorID, err := st.EnsureDevMember(ctx, cfg.DevMember)
	if err != nil {
		log.Fatal(err)
	}
	if cfg.DevAuthEnabled {
		if err := st.GrantAllPermissions(ctx, devActorID); err != nil {
			log.Fatal(err)
		}
	}

	masterKey, err := adminauth.LoadMasterKey(cfg.DataDir, cfg.MasterKey)
	if err != nil {
		log.Fatal(err)
	}
	admin := adminauth.New(pm.SystemDB, masterKey)
	if err := admin.EnsureBootstrapAdmin(ctx, cfg.AdminUsername, cfg.AdminBootstrapPassword); err != nil {
		log.Fatal(err)
	}

	app, err := httpserver.New(pm, st, admin, cfg, devActorID)
	if err != nil {
		log.Fatal(err)
	}
	handler := httpserver.WithTOTPSetupAlias(app, app.Handler())
	srv := &http.Server{Addr: cfg.Addr, Handler: handler, ReadHeaderTimeout: 5 * time.Second}
	go func() {
		log.Printf("FmlySys listening on %s, partition=%s, config=%s", cfg.Addr, pm.ActiveID, cfg.ConfigFile)
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
