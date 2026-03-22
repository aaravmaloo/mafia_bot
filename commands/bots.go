package commands

import (
	"context"
	"fmt"
	"log"
	"math/rand"
	"strings"
	"time"

	"mafia-bot/game"
	"mafia-bot/models"
)

func (r *Router) routeBots(ctx context.Context, msg InboundMessage, args string) {
	if !msg.IsGroup {
		return
	}

	mode := strings.ToLower(strings.TrimSpace(args))
	switch mode {
	case "enable":
		r.games.SetBotsEnabled(msg.ChatJID, true)
		if state := r.games.Get(msg.ChatJID); state != nil {
			state.SetBotsEnabled(true)
		}
		r.replyError(ctx, msg, fmt.Sprintf("Bot seats enabled for this group. %s will auto-fill missing seats up to %d.", formatCommand(r.cfg.Prefix, "begin"), r.cfg.MinPlayers))
	case "disable":
		r.games.SetBotsEnabled(msg.ChatJID, false)
		if state := r.games.Get(msg.ChatJID); state != nil {
			state.SetBotsEnabled(false)
			if removed := state.RemoveBotPlayers(); removed > 0 {
				r.replyError(ctx, msg, fmt.Sprintf("Bot seats disabled. Removed %d bot seats from the lobby.", removed))
				return
			}
		}
		r.replyError(ctx, msg, "Bot seats disabled for this group.")
	default:
		r.replyError(ctx, msg, fmt.Sprintf("Use %s or %s.", formatCommand(r.cfg.Prefix, "bots enable"), formatCommand(r.cfg.Prefix, "bots disable")))
	}
}

func (s *Service) queueBotActions(state *game.GameState, phase models.Phase) {
	if state.BotPlayerCount() == 0 {
		return
	}

	groupKey := state.GroupKey()
	switch phase {
	case models.PhaseNight:
		go func() {
			time.Sleep(1500 * time.Millisecond)
			s.runBotNight(context.Background(), groupKey)
		}()
	case models.PhaseDay:
		go func() {
			time.Sleep(3 * time.Second)
			s.runBotDay(context.Background(), groupKey)
		}()
	case models.PhaseVoting:
		go func() {
			time.Sleep(2 * time.Second)
			s.runBotVoting(context.Background(), groupKey)
		}()
	}
}

func (s *Service) runBotNight(ctx context.Context, groupKey string) {
	state := s.games.GetByKey(groupKey)
	if state == nil || state.Phase() != models.PhaseNight {
		return
	}

	alive := state.AlivePlayers()
	mafiaBots := make([]*models.Player, 0)
	doctorBots := make([]*models.Player, 0)
	policeBots := make([]*models.Player, 0)
	nonMafiaTargets := make([]*models.Player, 0)

	for _, player := range alive {
		if player.Role != models.RoleMafia {
			nonMafiaTargets = append(nonMafiaTargets, player)
		}
		if !player.IsBot {
			continue
		}
		switch player.Role {
		case models.RoleMafia:
			mafiaBots = append(mafiaBots, player)
		case models.RoleDoctor:
			doctorBots = append(doctorBots, player)
		case models.RolePolice:
			policeBots = append(policeBots, player)
		}
	}

	var mafiaTarget *models.Player
	if len(mafiaBots) > 0 && len(nonMafiaTargets) > 0 {
		mafiaTarget = chooseRandomPlayer(nonMafiaTargets)
		for _, actor := range mafiaBots {
			if err := game.ProcessKill(state, game.NormalizeJID(actor.JID), game.NormalizeJID(mafiaTarget.JID)); err == nil {
				log.Printf("bot night action group=%s actor=%s action=kill target=%s", state.Game.GroupJID, actor.Name, mafiaTarget.Name)
			}
		}
	}

	if len(alive) > 0 {
		for _, actor := range doctorBots {
			target := chooseRandomPlayer(alive)
			if target == nil {
				continue
			}
			if err := game.ProcessSave(state, game.NormalizeJID(actor.JID), game.NormalizeJID(target.JID)); err == nil {
				log.Printf("bot night action group=%s actor=%s action=save target=%s", state.Game.GroupJID, actor.Name, target.Name)
			}
		}
	}

	for _, actor := range policeBots {
		targets := make([]*models.Player, 0, len(alive))
		for _, candidate := range alive {
			if candidate.JID.ToNonAD() != actor.JID.ToNonAD() {
				targets = append(targets, candidate)
			}
		}
		target := chooseRandomPlayer(targets)
		if target == nil {
			continue
		}
		checked, role, err := game.ProcessInvestigate(state, game.NormalizeJID(actor.JID), game.NormalizeJID(target.JID))
		if err == nil {
			log.Printf("bot night action group=%s actor=%s action=investigate target=%s result=%s", state.Game.GroupJID, actor.Name, checked.Name, role)
		}
	}

	if game.AllNightActionsReceived(state) {
		s.finishNight(ctx, groupKey)
	}
}

func (s *Service) runBotDay(ctx context.Context, groupKey string) {
	state := s.games.GetByKey(groupKey)
	if state == nil || state.Phase() != models.PhaseDay {
		return
	}

	aliveBots := state.BotPlayers(true)
	if len(aliveBots) == 0 {
		return
	}

	alivePlayers := state.AlivePlayers()
	for _, actor := range aliveBots {
		targets := make([]*models.Player, 0, len(alivePlayers))
		for _, candidate := range alivePlayers {
			if candidate.JID.ToNonAD() == actor.JID.ToNonAD() {
				continue
			}
			if actor.Role == models.RoleMafia && candidate.Role == models.RoleMafia {
				continue
			}
			targets = append(targets, candidate)
		}
		target := chooseRandomPlayer(targets)
		if target == nil {
			continue
		}
		if err := s.SubmitNomination(ctx, state, actor, target); err == nil {
			log.Printf("bot day action group=%s actor=%s action=nominate target=%s", state.Game.GroupJID, actor.Name, target.Name)
		}
	}
}

func (s *Service) runBotVoting(ctx context.Context, groupKey string) {
	state := s.games.GetByKey(groupKey)
	if state == nil || state.Phase() != models.PhaseVoting {
		return
	}

	target := state.GetPlayerByKey(state.GetTrialTarget())
	if target == nil {
		return
	}

	for _, actor := range state.BotPlayers(true) {
		guilty := chooseBotVerdict(actor, target)
		if err := s.SubmitVerdict(ctx, state, actor, guilty); err == nil {
			log.Printf("bot voting action group=%s actor=%s guilty=%t target=%s", state.Game.GroupJID, actor.Name, guilty, target.Name)
		}
	}
}

func chooseRandomPlayer(players []*models.Player) *models.Player {
	if len(players) == 0 {
		return nil
	}
	return players[rand.Intn(len(players))]
}

func chooseBotVerdict(actor, target *models.Player) bool {
	switch {
	case actor.Role == models.RoleMafia && target.Role != models.RoleMafia:
		return false
	case actor.Role != models.RoleMafia && target.Role == models.RoleMafia:
		return true
	default:
		return rand.Intn(100) < 55
	}
}
