package commands

import (
	"context"
	"strings"

	"mafia-bot/config"
	"mafia-bot/game"
	"mafia-bot/models"

	"go.mau.fi/whatsmeow/types"
)

type Messenger interface {
	SendGroup(ctx context.Context, groupJID types.JID, text string) error
	SendDM(ctx context.Context, userJID types.JID, text string) error
	SendButtons(ctx context.Context, chatJID types.JID, text string, buttons []string) error
	DeleteMsg(ctx context.Context, chatJID, senderJID types.JID, messageID types.MessageID) error
}

type InboundMessage struct {
	ChatJID   types.JID
	SenderJID types.JID
	MessageID types.MessageID
	PushName  string
	Text      string
	IsGroup   bool
	Mentions  []types.JID
}

type Router struct {
	cfg             config.AppConfig
	allowedGroupKey string
	games           *game.Registry
	service         *Service
}

type Service struct {
	cfg       config.AppConfig
	games     *game.Registry
	messenger Messenger
}

type command struct {
	name string
	args string
}

func NewRouter(cfg config.AppConfig, allowedGroup types.JID, games *game.Registry, messenger Messenger) *Router {
	allowedKey := ""
	if !allowedGroup.IsEmpty() {
		allowedKey = game.NormalizeJID(allowedGroup)
	}

	service := &Service{
		cfg:       cfg,
		games:     games,
		messenger: messenger,
	}

	return &Router{
		cfg:             cfg,
		allowedGroupKey: allowedKey,
		games:           games,
		service:         service,
	}
}

func (r *Router) HandleMessage(ctx context.Context, msg InboundMessage) {
	cmd, ok := parseCommand(r.cfg.Prefix, msg.Text)
	if !ok {
		return
	}

	if msg.IsGroup && !r.groupAllowed(msg.ChatJID) {
		return
	}

	switch cmd.name {
	case "startgame":
		r.routeStartGame(ctx, msg)
	case "join":
		r.routeJoin(ctx, msg)
	case "begin":
		r.routeBegin(ctx, msg)
	case "endgame":
		r.routeEndGame(ctx, msg)
	case "kill":
		if msg.IsGroup {
			return
		}
		r.routeKill(ctx, msg, cmd.args)
	case "save":
		if msg.IsGroup {
			return
		}
		r.routeSave(ctx, msg, cmd.args)
	case "investigate":
		if msg.IsGroup {
			return
		}
		r.routeInvestigate(ctx, msg, cmd.args)
	case "nominate":
		r.routeNominate(ctx, msg, cmd.args)
	case "guilty":
		r.routeVerdict(ctx, msg, true)
	case "notguilty":
		r.routeVerdict(ctx, msg, false)
	}
}

func parseCommand(prefix, raw string) (command, bool) {
	text := strings.TrimSpace(raw)
	if text == "" || !strings.HasPrefix(text, prefix) {
		return command{}, false
	}

	body := strings.TrimSpace(strings.TrimPrefix(text, prefix))
	if body == "" {
		return command{}, false
	}

	parts := strings.Fields(body)
	name := strings.ToLower(parts[0])
	args := ""
	if len(parts) > 1 {
		args = strings.TrimSpace(strings.TrimPrefix(body, parts[0]))
	}

	return command{name: name, args: args}, true
}

func (r *Router) groupAllowed(groupJID types.JID) bool {
	if r.allowedGroupKey == "" {
		return true
	}
	return r.allowedGroupKey == game.NormalizeJID(groupJID)
}

func (r *Router) replyError(ctx context.Context, msg InboundMessage, text string) {
	if msg.IsGroup {
		_ = r.service.messenger.SendGroup(ctx, msg.ChatJID, text)
		return
	}
	_ = r.service.messenger.SendDM(ctx, msg.SenderJID, text)
}

func (r *Router) lookupPlayerGame(msg InboundMessage) (*game.GameState, *models.Player) {
	state := r.games.FindByPlayer(msg.SenderJID)
	if state == nil {
		return nil, nil
	}
	return state, state.GetPlayer(msg.SenderJID)
}
