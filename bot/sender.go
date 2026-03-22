package bot

import (
	"context"
	"fmt"
	"log"
	"strings"

	"go.mau.fi/whatsmeow"
	waProto "go.mau.fi/whatsmeow/proto/waE2E"
	"go.mau.fi/whatsmeow/types"
	"google.golang.org/protobuf/proto"
)

type Sender struct {
	client *whatsmeow.Client
}

func NewSender(client *whatsmeow.Client) *Sender {
	return &Sender{client: client}
}

func (s *Sender) SendGroup(ctx context.Context, groupJID types.JID, text string) error {
	log.Printf("send group chat=%s text=%q", groupJID, strings.TrimSpace(text))
	return s.sendText(ctx, groupJID, text)
}

func (s *Sender) SendDM(ctx context.Context, userJID types.JID, text string) error {
	log.Printf("send dm user=%s text=%q", userJID, strings.TrimSpace(text))
	return s.sendText(ctx, userJID, text)
}

func (s *Sender) SendButtons(ctx context.Context, chatJID types.JID, text string, buttons []string) error {
	lines := []string{text}
	for index, button := range buttons {
		lines = append(lines, fmt.Sprintf("%d. %s", index+1, button))
	}
	return s.sendText(ctx, chatJID, strings.Join(lines, "\n"))
}

func (s *Sender) DeleteMsg(ctx context.Context, chatJID, senderJID types.JID, messageID types.MessageID) error {
	log.Printf("delete message chat=%s sender=%s id=%s", chatJID, senderJID, messageID)
	_, err := s.client.SendMessage(ctx, chatJID, s.client.BuildRevoke(chatJID, senderJID, messageID))
	if err != nil {
		log.Printf("delete failed chat=%s sender=%s id=%s err=%v", chatJID, senderJID, messageID, err)
	}
	return err
}

func (s *Sender) sendText(ctx context.Context, chatJID types.JID, text string) error {
	body := strings.TrimSpace(text)
	if body == "" {
		return nil
	}

	message := &waProto.Message{
		Conversation: proto.String(formatBotMessage(body)),
	}
	_, err := s.client.SendMessage(ctx, chatJID, message)
	if err != nil {
		log.Printf("send failed chat=%s err=%v", chatJID, err)
	}
	return err
}

func formatBotMessage(text string) string {
	return fmt.Sprintf("[*BOT*]: %s", strings.TrimSpace(text))
}
