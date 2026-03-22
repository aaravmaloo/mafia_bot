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
		r.replyError(ctx, msg, "No lobby is open. Ask the host to use !startgame first.")
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
		r.replyError(ctx, msg, "Only the host can use !begin.")
		return
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
		r.replyError(ctx, msg, "Only the host can use !endgame.")
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
		fmt.Sprintf("🎭 Mafia lobby opened by %s.", nameForDisplay(msg.PushName, msg.SenderJID.User)),
		fmt.Sprintf("Need at least %d players.", s.cfg.MinPlayers),
		"Use !join to enter the game.",
		"Host uses !begin when everyone's in.",
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

	return s.messenger.SendGroup(ctx, state.Game.GroupJID, fmt.Sprintf("%s joined the lobby. Players: %d", player.Name, state.PlayerCount()))
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
		if player == nil {
			continue
		}
		if err := s.messenger.SendDM(ctx, player.JID, dm.Text); err != nil {
			return err
		}
	}

	if err := s.messenger.SendGroup(ctx, state.Game.GroupJID, "Roles are out. Check your DMs."); err != nil {
		return err
	}

	return s.applyBundle(ctx, state, game.StartNight(state, s.cfg.NightDuration))
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
