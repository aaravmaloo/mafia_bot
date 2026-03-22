package store

import (
	"context"
	"fmt"
	"log"
	"net/url"
	"os"
	"path/filepath"
	"strings"

	_ "github.com/mattn/go-sqlite3"
	waStore "go.mau.fi/whatsmeow/store"
	"go.mau.fi/whatsmeow/store/sqlstore"
	waLog "go.mau.fi/whatsmeow/util/log"
)

func OpenDeviceStore(ctx context.Context, driver, dsn, logLevel string) (*sqlstore.Container, *waStore.Device, error) {
	resolvedDSN, err := resolveDSN(driver, dsn)
	if err != nil {
		return nil, nil, fmt.Errorf("resolve session database path: %w", err)
	}

	dbLog := waLog.Stdout("Database", logLevel, true)
	log.Printf("opening whatsmeow store driver=%s dsn=%q", driver, resolvedDSN)

	container, err := sqlstore.New(ctx, driver, resolvedDSN, dbLog)
	if err != nil {
		return nil, nil, fmt.Errorf("open session database: %w", err)
	}

	deviceStore, err := container.GetFirstDevice(ctx)
	if err != nil {
		_ = container.Close()
		return nil, nil, fmt.Errorf("load device store: %w", err)
	}

	return container, deviceStore, nil
}

func resolveDSN(driver, dsn string) (string, error) {
	if strings.TrimSpace(driver) != "sqlite3" {
		return dsn, nil
	}

	trimmed := strings.TrimSpace(dsn)
	if trimmed == "" {
		trimmed = "mafia-bot.db?_foreign_keys=on"
	}
	if trimmed == ":memory:" || strings.Contains(trimmed, "mode=memory") {
		return trimmed, nil
	}

	filename := trimmed
	query := ""
	if idx := strings.Index(trimmed, "?"); idx >= 0 {
		filename = trimmed[:idx]
		query = trimmed[idx+1:]
	}
	filename = strings.TrimPrefix(filename, "file:")
	filename = strings.TrimSpace(filename)
	if filename == "" {
		filename = "mafia-bot.db"
	}

	absPath, err := filepath.Abs(filename)
	if err != nil {
		return "", err
	}
	if err = os.MkdirAll(filepath.Dir(absPath), 0o755); err != nil {
		return "", err
	}

	values, err := url.ParseQuery(query)
	if err != nil {
		return "", err
	}
	if values.Get("_foreign_keys") == "" {
		values.Set("_foreign_keys", "on")
	}
	if values.Get("_busy_timeout") == "" {
		values.Set("_busy_timeout", "10000")
	}
	if values.Get("_journal_mode") == "" {
		values.Set("_journal_mode", "WAL")
	}
	if values.Get("_synchronous") == "" {
		values.Set("_synchronous", "NORMAL")
	}

	encoded := values.Encode()
	if encoded == "" {
		return absPath, nil
	}
	return absPath + "?" + encoded, nil
}
