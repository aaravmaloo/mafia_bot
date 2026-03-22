package bot

import (
	"context"
	"log"
	"strings"

	"mafia-bot/commands"
	"mafia-bot/game"

	waProto "go.mau.fi/whatsmeow/proto/waE2E"
	"go.mau.fi/whatsmeow/types"
	"go.mau.fi/whatsmeow/types/events"
)

type Handler struct {
	client          *Client
	sender          *Sender
	router          *commands.Router
	games           *game.Registry
	allowedGroupKey string
}

func NewHandler(client *Client, sender *Sender, router *commands.Router, games *game.Registry, allowedGroup types.JID) *Handler {
	allowedKey := ""
	if !allowedGroup.IsEmpty() {
		allowedKey = game.NormalizeJID(allowedGroup)
	}

	return &Handler{
		client:          client,
		sender:          sender,
		router:          router,
		games:           games,
		allowedGroupKey: allowedKey,
	}
}

func (h *Handler) HandleEvent(evt any) {
	switch event := evt.(type) {
	case *events.Message:
		h.handleMessage(event)
	case *events.Connected:
		log.Printf("whatsapp connected")
	case *events.Disconnected:
		log.Printf("whatsapp disconnected")
		h.client.ScheduleReconnect()
	case *events.LoggedOut:
		log.Printf("whatsapp logged out: %s", event.PermanentDisconnectDescription())
	}
}

func (h *Handler) handleMessage(evt *events.Message) {
	if evt.IsEdit || evt.Message == nil {
		return
	}

	if evt.Info.IsGroup && h.allowedGroupKey != "" && game.NormalizeJID(evt.Info.Chat) != h.allowedGroupKey {
		return
	}

	if h.shouldBlockDeadPlayer(evt) {
		return
	}

	text := extractText(evt.Message)
	if strings.TrimSpace(text) == "" {
		return
	}
	if evt.Info.IsFromMe && strings.HasPrefix(strings.TrimSpace(text), "[*BOT*]:") {
		return
	}
	log.Printf("incoming message chat=%s sender=%s group=%t text=%q", evt.Info.Chat, evt.Info.Sender, evt.Info.IsGroup, text)

	inbound := commands.InboundMessage{
		ChatJID:   evt.Info.Chat,
		SenderJID: evt.Info.Sender,
		MessageID: evt.Info.ID,
		PushName:  evt.Info.PushName,
		Text:      text,
		IsGroup:   evt.Info.IsGroup,
		Mentions:  extractMentions(evt.Message),
	}
	h.router.HandleMessage(context.Background(), inbound)
}

func (h *Handler) shouldBlockDeadPlayer(evt *events.Message) bool {
	var state *game.GameState
	if evt.Info.IsGroup {
		state = h.games.Get(evt.Info.Chat)
	} else {
		state = h.games.FindByPlayer(evt.Info.Sender)
	}
	if state == nil {
		return false
	}

	player := state.GetPlayer(evt.Info.Sender)
	if player == nil || player.Alive {
		return false
	}

	if evt.Info.IsGroup {
		log.Printf("dead player message blocked sender=%s chat=%s", evt.Info.Sender, evt.Info.Chat)
		_ = h.sender.DeleteMsg(context.Background(), evt.Info.Chat, evt.Info.Sender, evt.Info.ID)
	}
	return true
}

func extractText(message *waProto.Message) string {
	switch {
	case message.GetConversation() != "":
		return message.GetConversation()
	case message.GetExtendedTextMessage() != nil:
		return message.GetExtendedTextMessage().GetText()
	case message.GetImageMessage() != nil:
		return message.GetImageMessage().GetCaption()
	case message.GetVideoMessage() != nil:
		return message.GetVideoMessage().GetCaption()
	case message.GetDocumentMessage() != nil:
		return message.GetDocumentMessage().GetCaption()
	default:
		return ""
	}
}

func extractMentions(message *waProto.Message) []types.JID {
	rawMentions := make([]string, 0)
	switch {
	case message.GetExtendedTextMessage() != nil && message.GetExtendedTextMessage().GetContextInfo() != nil:
		rawMentions = message.GetExtendedTextMessage().GetContextInfo().GetMentionedJID()
	case message.GetImageMessage() != nil && message.GetImageMessage().GetContextInfo() != nil:
		rawMentions = message.GetImageMessage().GetContextInfo().GetMentionedJID()
	case message.GetVideoMessage() != nil && message.GetVideoMessage().GetContextInfo() != nil:
		rawMentions = message.GetVideoMessage().GetContextInfo().GetMentionedJID()
	case message.GetDocumentMessage() != nil && message.GetDocumentMessage().GetContextInfo() != nil:
		rawMentions = message.GetDocumentMessage().GetContextInfo().GetMentionedJID()
	}

	mentions := make([]types.JID, 0, len(rawMentions))
	for _, raw := range rawMentions {
		jid, err := types.ParseJID(raw)
		if err == nil {
			mentions = append(mentions, jid)
		}
	}
	return mentions
}
