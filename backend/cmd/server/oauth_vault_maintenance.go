package main

import (
	"context"
	"database/sql"
	"fmt"
	"time"

	"anlapi/internal/config"
	"anlapi/internal/repository"
	_ "github.com/lib/pq"
)

func runOAuthVaultMaintenance(apply bool) error {
	cfg, err := config.Load()
	if err != nil {
		return fmt.Errorf("load config: %w", err)
	}
	vault, err := repository.NewOAuthCredentialVault(cfg)
	if err != nil {
		return err
	}
	defer func() { _ = vault.Close() }()

	db, err := sql.Open("postgres", cfg.Database.DSN())
	if err != nil {
		return fmt.Errorf("open business database: %w", err)
	}
	defer func() { _ = db.Close() }()

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Minute)
	defer cancel()
	if err := db.PingContext(ctx); err != nil {
		return fmt.Errorf("ping business database: %w", err)
	}

	var report *repository.OAuthCredentialIsolationReport
	if apply {
		report, err = repository.MigrateOAuthCredentialsToVault(ctx, db, vault)
	} else {
		report, err = repository.AuditOAuthCredentialIsolation(ctx, db, vault)
	}
	if err != nil {
		return err
	}
	fmt.Printf(
		"oauth_vault accounts=%d sensitive_rows=%d marker_rows=%d missing_vault_entries=%d migrated_rows=%d\n",
		report.Accounts,
		report.SensitiveRows,
		report.MarkerRows,
		report.MissingVaultEntries,
		report.MigratedRows,
	)
	return nil
}
