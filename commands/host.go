package commands

import (
	"context"
	"fmt"
	"log"
	"strings"

	"mafia-bot/game"
	"mafia-bot/models"
)

func (r *Router) routeStartGame(ctx context.Context, msg InboundMessage) {
	if !msg.IsGroup {
		return
	}
	if state := r.games.Get(msg.ChatJID); state != nil {
		r.replyError(ctx, msg, "A game is already running in this group.")
		return
	}
	if err := r.service.StartLobby(ctx, msg); err != nil {
		r.replyError(ctx, msg, err.Error())
	}
}

func (r *Router) routeJoin(ctx context.Context, msg InboundMessage) {
	if !msg.IsGroup {
		return
	}

	state := r.games.Get(msg.ChatJID)
	if state == nil {
		r.replyError(ctx, msg, fmt.Sprintf("No lobby is open. Ask the host to use %s first.", formatCommand(r.cfg.Prefix, "startgame")))
		return
	}
	if state.Phase() != models.PhaseLobby {
		r.replyError(ctx, msg, "You can't join once the game has already started.")
		return
	}

	if err := r.service.JoinLobby(ctx, state, msg); err != nil {
		r.replyError(ctx, msg, err.Error())
	}
}

func (r *Router) routeBegin(ctx context.Context, msg InboundMessage) {
	if !msg.IsGroup {
		return
	}

	state := r.games.Get(msg.ChatJID)
	if state == nil {
		r.replyError(ctx, msg, "No lobby is open right now.")
		return
	}
	if state.Phase() != models.PhaseLobby {
		r.replyError(ctx, msg, "This game has already begun.")
		return
	}
	if state.HostKey() != game.NormalizeJID(msg.SenderJID) {
		r.replyError(ctx, msg, fmt.Sprintf("Only the host can use %s.", formatCommand(r.cfg.Prefix, "begin")))
		return
	}
	if state.HumanPlayerCount() == 0 {
		r.replyError(ctx, msg, "At least one real player needs to join the lobby first.")
		return
	}
	if state.BotsEnabled() && state.PlayerCount() < r.cfg.MinPlayers {
		added := game.FillBotSeats(state, r.cfg.MinPlayers)
		if len(added) > 0 {
			names := make([]string, 0, len(added))
			for _, player := range added {
				names = append(names, player.Name)
			}
			log.Printf("filled bot seats group=%s added=%v", state.Game.GroupJID, names)
			_ = r.service.messenger.SendGroup(ctx, state.Game.GroupJID, fmt.Sprintf("Filled %d empty seats with bots: %s", len(added), strings.Join(names, ", ")))
		}
	}
	if state.PlayerCount() < r.cfg.MinPlayers {
		r.replyError(ctx, msg, fmt.Sprintf("Need at least %d players before the game can begin.", r.cfg.MinPlayers))
		return
	}

	if err := r.service.BeginGame(ctx, state); err != nil {
		r.replyError(ctx, msg, err.Error())
	}
}

func (r *Router) routeEndGame(ctx context.Context, msg InboundMessage) {
	if !msg.IsGroup {
		return
	}

	state := r.games.Get(msg.ChatJID)
	if state == nil {
		r.replyError(ctx, msg, "There's no active game to end.")
		return
	}
	if state.HostKey() != game.NormalizeJID(msg.SenderJID) {
		r.replyError(ctx, msg, fmt.Sprintf("Only the host can use %s.", formatCommand(r.cfg.Prefix, "endgame")))
		return
	}

	if err := r.service.ForceEndGame(ctx, state, "Host ended the game."); err != nil {
		r.replyError(ctx, msg, err.Error())
	}
}

func (s *Service) StartLobby(ctx context.Context, msg InboundMessage) error {
	state, err := s.games.Create(msg.ChatJID, msg.SenderJID)
	if err != nil {
		return err
	}
	log.Printf("lobby opened group=%s host=%s", state.Game.GroupJID, msg.SenderJID)

	announcement := strings.Join([]string{
		"━━━━━━━━━━━━━━━━━━━━",
		"🎭 MAFIA LOBBY",
		fmt.Sprintf("Host: %s", nameForDisplay(msg.PushName, msg.SenderJID.User)),
		fmt.Sprintf("Minimum players: %d", s.cfg.MinPlayers),
		fmt.Sprintf("Join: %s", formatCommand(s.cfg.Prefix, "join")),
		fmt.Sprintf("Bot seats: %s / %s", formatCommand(s.cfg.Prefix, "bots enable"), formatCommand(s.cfg.Prefix, "bots disable")),
		fmt.Sprintf("Start: %s", formatCommand(s.cfg.Prefix, "begin")),
		"━━━━━━━━━━━━━━━━━━━━",
	}, "\n")

	return s.applyBundle(ctx, state, game.PhaseBundle{
		Phase:     models.PhaseLobby,
		GroupText: announcement,
	})
}

func (s *Service) JoinLobby(ctx context.Context, state *game.GameState, msg InboundMessage) error {
	player, err := state.AddPlayer(msg.SenderJID, msg.PushName)
	if err != nil {
		return err
	}
	log.Printf("player joined group=%s player=%s total=%d", state.Game.GroupJID, player.Name, state.PlayerCount())

	return s.messenger.SendGroup(ctx, state.Game.GroupJID, fmt.Sprintf("✅ %s joined the lobby.\nSeats filled: %d", player.Name, state.PlayerCount()))
}

func (s *Service) BeginGame(ctx context.Context, state *game.GameState) error {
	if err := game.AssignRoles(state); err != nil {
		return err
	}
	log.Printf("game beginning group=%s players=%d", state.Game.GroupJID, state.PlayerCount())
	for _, player := range state.JoinedPlayers() {
		log.Printf("role assigned group=%s player=%s role=%s", state.Game.GroupJID, player.Name, player.Role)
	}

	for _, dm := range game.RoleRevealMessages(state) {
		player := state.GetPlayerByKey(dm.RecipientKey)
		if player == nil || player.IsBot {
			continue
		}
		if err := s.messenger.SendDM(ctx, player.JID, dm.Text); err != nil {
			return err
		}
	}

	if err := s.messenger.SendGroup(ctx, state.Game.GroupJID, "━━━━━━━━━━━━━━━━━━━━\n🎲 Roles are out. Check your DMs.\n━━━━━━━━━━━━━━━━━━━━"); err != nil {
		return err
	}

	return s.applyBundle(ctx, state, game.StartNight(state, s.cfg.NightDuration, s.cfg.Prefix))
}

func (s *Service) ForceEndGame(ctx context.Context, state *game.GameState, reason string) error {
	game.CancelTimer(state)
	log.Printf("game force ended group=%s reason=%q", state.Game.GroupJID, reason)
	if strings.TrimSpace(reason) != "" {
		if err := s.messenger.SendGroup(ctx, state.Game.GroupJID, reason); err != nil {
			return err
		}
	}
	bundle := game.EndGame(state, "Host ended the game.")
	if err := s.applyBundle(ctx, state, bundle); err != nil {
		return err
	}
	s.games.Delete(state.Game.GroupJID)
	return nil
}

func nameForDisplay(name, fallback string) string {
	name = strings.TrimSpace(name)
	if name != "" {
		return name
	}
	if strings.TrimSpace(fallback) != "" {
		return fallback
	}
	return "Unknown"
}
