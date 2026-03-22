package store

import (
	"context"
	"fmt"

	_ "github.com/mattn/go-sqlite3"
	waStore "go.mau.fi/whatsmeow/store"
	"go.mau.fi/whatsmeow/store/sqlstore"
	waLog "go.mau.fi/whatsmeow/util/log"
)

func OpenDeviceStore(ctx context.Context, driver, dsn, logLevel string) (*sqlstore.Container, *waStore.Device, error) {
	dbLog := waLog.Stdout("Database", logLevel, true)

	container, err := sqlstore.New(ctx, driver, dsn, dbLog)
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
