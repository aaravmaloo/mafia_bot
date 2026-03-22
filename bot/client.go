package bot

import (
	"context"
	"fmt"
	"log"
	"os"
	"sync"
	"time"

	"github.com/mdp/qrterminal/v3"
	"go.mau.fi/whatsmeow"
	waStore "go.mau.fi/whatsmeow/store"
	waLog "go.mau.fi/whatsmeow/util/log"
)

type Client struct {
	WA *whatsmeow.Client

	mu           sync.Mutex
	reconnecting bool
}

func NewClient(deviceStore *waStore.Device, logLevel string) *Client {
	clientLog := waLog.Stdout("Client", logLevel, true)
	return &Client{
		WA: whatsmeow.NewClient(deviceStore, clientLog),
	}
}

func (c *Client) Connect(ctx context.Context) error {
	log.Printf("whatsapp connect requested")
	if c.WA.Store.ID == nil {
		qrChan, err := c.WA.GetQRChannel(ctx)
		if err != nil {
			return fmt.Errorf("create QR channel: %w", err)
		}
		if err = c.WA.Connect(); err != nil {
			return fmt.Errorf("connect whatsapp client: %w", err)
		}

		for evt := range qrChan {
			if evt.Event == "code" {
				log.Printf("new QR code generated")
				qrterminal.GenerateHalfBlock(evt.Code, qrterminal.L, os.Stdout)
				continue
			}
			log.Printf("whatsapp login event=%s", evt.Event)
		}
		return nil
	}

	if err := c.WA.Connect(); err != nil {
		return fmt.Errorf("connect whatsapp client: %w", err)
	}
	return nil
}

func (c *Client) Disconnect() {
	c.WA.Disconnect()
}

func (c *Client) ScheduleReconnect() {
	c.mu.Lock()
	if c.reconnecting {
		c.mu.Unlock()
		return
	}
	c.reconnecting = true
	c.mu.Unlock()

	log.Printf("scheduling reconnect in 5s")

	go func() {
		defer func() {
			c.mu.Lock()
			c.reconnecting = false
			c.mu.Unlock()
		}()

		time.Sleep(5 * time.Second)
		if c.WA.Store.ID == nil || c.WA.IsConnected() {
			return
		}
		if err := c.WA.Connect(); err != nil {
			log.Printf("reconnect failed: %v", err)
			return
		}
		log.Printf("reconnect succeeded")
	}()
}
