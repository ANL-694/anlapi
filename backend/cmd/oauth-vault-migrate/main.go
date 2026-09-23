// oauth-vault-migrate is an explicit maintenance command. It never falls back
// to the business database when the external Vault is unavailable.
package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"time"

	_ "github.com/Wei-Shaw/sub2api/ent/runtime"
	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/Wei-Shaw/sub2api/internal/repository"
	"github.com/Wei-Shaw/sub2api/internal/service"
)

func main() {
	execute := flag.Bool("execute", false, "move legacy OAuth bearer credentials into the external Vault (default: audit only)")
	reconcile := flag.Bool("reconcile", false, "remove unreferenced Vault versions older than the grace period and retry durable deletes")
	grace := flag.Duration("reconcile-grace", 15*time.Minute, "minimum age before an unreferenced Vault version can be removed")
	flag.Parse()
	if *execute && *reconcile {
		log.Fatal("--execute and --reconcile cannot be used together")
	}
	if *grace <= 0 {
		log.Fatal("--reconcile-grace must be positive")
	}

	cfg, err := config.LoadForBootstrap()
	if err != nil {
		log.Fatalf("load config: %v", err)
	}
	if cfg.OAuthVault.Mode != string(service.OAuthCredentialVaultModeExternal) {
		log.Fatal("oauth_vault.mode=external is required")
	}
	client, db, err := repository.InitEnt(cfg)
	if err != nil {
		log.Fatalf("initialize business database: %v", err)
	}
	defer func() { _ = client.Close() }()
	vault, err := repository.NewOAuthCredentialVault(cfg)
	if err != nil {
		log.Fatalf("initialize OAuth credential Vault: %v", err)
	}
	defer func() { _ = vault.Close() }()

	ctx := context.Background()
	var report *repository.OAuthCredentialIsolationReport
	if *execute {
		report, err = repository.MigrateOAuthCredentialsToVault(ctx, db, vault)
	} else if *reconcile {
		report, err = repository.ReconcileOAuthCredentialVault(ctx, db, vault, time.Now().Add(-*grace))
	} else {
		report, err = repository.AuditOAuthCredentialIsolation(ctx, db, vault)
	}
	if err != nil {
		log.Fatalf("OAuth credential Vault maintenance failed: %v", err)
	}
	mode := "audit"
	if *execute {
		mode = "execute"
	} else if *reconcile {
		mode = "reconcile"
	}
	fmt.Printf("mode=%s accounts=%d sensitive_rows=%d marker_rows=%d missing_vault_entries=%d migrated_rows=%d reclaimed_versions=%d completed_deletes=%d pending_deletes=%d pending_writes=%d\n",
		mode,
		report.Accounts,
		report.SensitiveRows,
		report.MarkerRows,
		report.MissingVaultEntries,
		report.MigratedRows,
		report.ReclaimedVersions,
		report.CompletedDeletes,
		report.PendingDeletes,
		report.PendingWrites,
	)
}
