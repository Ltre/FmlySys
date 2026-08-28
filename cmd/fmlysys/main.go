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
	// All server-side fallback formatting uses UTC+8. Browser pages then adopt
	// the device IANA timezone through fmly_timezone/timezone.js.
	if loc, err := time.LoadLocation("Asia/Shanghai"); err == nil {
		time.Local = loc
	} else {
		time.Local = time.FixedZone("UTC+8", 8*60*60)
	}

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

	masterKey, err := adminauth.LoadMasterKeyForStartup(cfg.DataDir, cfg.MasterKey, cfg.AdminBootstrapPassword)
	if err != nil {
		log.Fatal(err)
	}
	admin := adminauth.New(pm.SystemDB, masterKey, cfg.DataDir)
	if err := admin.EnsureBootstrapAdminRecoverable(ctx, cfg.AdminUsername, cfg.AdminBootstrapPassword); err != nil {
		log.Fatal(err)
	}

	app, err := httpserver.New(pm, st, admin, cfg, devActorID)
	if err != nil {
		log.Fatal(err)
	}
	reminderCtx, stopReminders := context.WithCancel(context.Background())
	defer stopReminders()
	app.StartMedicationReminderLoopV3(reminderCtx)

	handler := httpserver.WithTOTPSetupAlias(app, app.Handler())
	handler = app.WithAdminMemberDelete(handler)
	handler = app.WithPasskeys(handler)
	handler = app.WithPasskeyIdentities(handler)
	handler = app.WithPasskeyCredentialBindings(handler)
	handler = app.WithAssetWorkflowFixes(handler)
	handler = app.WithMoneyWorkflowV3(handler)
	handler = app.WithMoneyRecordDetails(handler)
	handler = app.WithQuickMoneyNotes(handler)
	handler = app.WithAdminEnhancements(handler)
	handler = app.WithMedicationEnhancements(handler)
	handler = app.WithMedicationV3(handler)
	handler = app.WithNotificationCenter(handler)
	handler = app.WithAuditConsole(handler)
	handler = app.WithAuditConsoleV2(handler)
	handler = app.WithPasskeyUnifiedLogin(handler)
	handler = app.WithPasskeyFrontDoorFixes(handler)
	handler = app.WithAdminSessionV2(handler)
	handler = httpserver.WithWeChatBrowserGuard(handler)
	handler = httpserver.WithRequestDeadline(handler, 15*time.Second)
	handler = httpserver.WithAsyncMultipartFormCompatibility(handler)
	handler = httpserver.WithEnhancedFormResponses(handler)
	handler = app.WithSuperAuditV2(handler)

	srv := &http.Server{Addr: cfg.Addr, Handler: handler, ReadHeaderTimeout: 5 * time.Second}
	go func() {
		log.Printf("FmlySys listening on %s, partition=%s, config=%s, admin_credentials=%s", cfg.Addr, pm.ActiveID, cfg.ConfigFile, admin.CredentialsPath())
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Fatal(err)
		}
	}()

	ch := make(chan os.Signal, 1)
	signal.Notify(ch, syscall.SIGTERM, syscall.SIGINT)
	<-ch
	stopReminders()
	shutdown, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	_ = srv.Shutdown(shutdown)
}
