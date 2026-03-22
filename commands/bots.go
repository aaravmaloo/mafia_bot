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
			time.Sleep(2 * time.Second)
			s.runBotDay(context.Background(), groupKey)
		}()
	case models.PhaseTrial:
		go func() {
			time.Sleep(2 * time.Second)
			s.runBotTrial(context.Background(), groupKey)
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
				s.notifyBotActionDM(ctx, state, fmt.Sprintf("%s secretly picked %s for the mafia kill.", actor.Name, mafiaTarget.Name))
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
				s.notifyBotActionDM(ctx, state, fmt.Sprintf("%s chose to protect %s tonight.", actor.Name, target.Name))
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
			s.notifyBotActionDM(ctx, state, fmt.Sprintf("%s investigated %s and got %s.", actor.Name, checked.Name, game.InvestigationResult(role)))
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
	speakers := min(3, len(aliveBots))
	for i := 0; i < speakers; i++ {
		actor := aliveBots[i]
		targets := make([]*models.Player, 0, len(alivePlayers))
		for _, candidate := range alivePlayers {
			if candidate.JID.ToNonAD() == actor.JID.ToNonAD() {
				continue
			}
			targets = append(targets, candidate)
		}
		target := chooseRandomPlayer(targets)
		if target == nil {
			continue
		}
		if err := s.messenger.SendGroup(ctx, state.Game.GroupJID, botDayLine(actor, target)); err == nil {
			log.Printf("bot day chat group=%s actor=%s target=%s", state.Game.GroupJID, actor.Name, target.Name)
		}
		time.Sleep(1200 * time.Millisecond)
		if state.Phase() != models.PhaseDay {
			return
		}
	}

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
			_ = s.messenger.SendGroup(ctx, state.Game.GroupJID, botNominationLine(actor, target))
			s.notifyBotActionDM(ctx, state, fmt.Sprintf("%s nominated %s.", actor.Name, target.Name))
			time.Sleep(900 * time.Millisecond)
		}
	}
}

func (s *Service) runBotTrial(ctx context.Context, groupKey string) {
	state := s.games.GetByKey(groupKey)
	if state == nil || state.Phase() != models.PhaseTrial {
		return
	}

	target := state.GetPlayerByKey(state.GetTrialTarget())
	if target == nil {
		return
	}

	if target.IsBot {
		_ = s.messenger.SendGroup(ctx, state.Game.GroupJID, botDefenseLine(target))
		log.Printf("bot trial defense group=%s actor=%s", state.Game.GroupJID, target.Name)
		return
	}

	bots := state.BotPlayers(true)
	if len(bots) == 0 {
		return
	}
	speaker := chooseRandomPlayer(bots)
	if speaker == nil {
		return
	}
	_ = s.messenger.SendGroup(ctx, state.Game.GroupJID, botTrialCommentLine(speaker, target))
	log.Printf("bot trial comment group=%s actor=%s target=%s", state.Game.GroupJID, speaker.Name, target.Name)
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
			_ = s.messenger.SendGroup(ctx, state.Game.GroupJID, botVoteLine(actor, target, guilty))
			s.notifyBotActionDM(ctx, state, fmt.Sprintf("%s voted %s on %s.", actor.Name, verdictWord(guilty), target.Name))
			time.Sleep(700 * time.Millisecond)
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

func botDayLine(actor, target *models.Player) string {
	lines := []string{
		fmt.Sprintf("%s: %s feels off today.", actor.Name, target.Name),
		fmt.Sprintf("%s: I'm watching %s. Something's weird there.", actor.Name, target.Name),
		fmt.Sprintf("%s: Lowkey I don't trust %s.", actor.Name, target.Name),
		fmt.Sprintf("%s: %s is talking like they know too much.", actor.Name, target.Name),
		fmt.Sprintf("%s: If we miss this, check %s tomorrow too.", actor.Name, target.Name),
	}
	return lines[rand.Intn(len(lines))]
}

func botNominationLine(actor, target *models.Player) string {
	lines := []string{
		fmt.Sprintf("%s: putting my vote on %s for now.", actor.Name, target.Name),
		fmt.Sprintf("%s: yeah I'm nominating %s.", actor.Name, target.Name),
		fmt.Sprintf("%s: %s is my best read right now.", actor.Name, target.Name),
	}
	return lines[rand.Intn(len(lines))]
}

func botDefenseLine(actor *models.Player) string {
	lines := []string{
		fmt.Sprintf("%s: this push on me is bad. You're hitting the wrong person.", actor.Name),
		fmt.Sprintf("%s: I'm not mafia, you're about to waste the day.", actor.Name),
		fmt.Sprintf("%s: if I flip innocent, remember who forced this trial.", actor.Name),
	}
	return lines[rand.Intn(len(lines))]
}

func botTrialCommentLine(actor, target *models.Player) string {
	lines := []string{
		fmt.Sprintf("%s: I still don't buy %s's story.", actor.Name, target.Name),
		fmt.Sprintf("%s: %s might be panicking right now.", actor.Name, target.Name),
		fmt.Sprintf("%s: listen carefully, %s is slipping.", actor.Name, target.Name),
	}
	return lines[rand.Intn(len(lines))]
}

func botVoteLine(actor, target *models.Player, guilty bool) string {
	if guilty {
		lines := []string{
			fmt.Sprintf("%s: guilty from me.", actor.Name),
			fmt.Sprintf("%s: I'm voting guilty on %s.", actor.Name, target.Name),
			fmt.Sprintf("%s: nah, %s goes.", actor.Name, target.Name),
		}
		return lines[rand.Intn(len(lines))]
	}

	lines := []string{
		fmt.Sprintf("%s: not guilty, this isn't enough.", actor.Name),
		fmt.Sprintf("%s: I'm keeping %s alive for now.", actor.Name, target.Name),
		fmt.Sprintf("%s: not guilty. wrong trial.", actor.Name),
	}
	return lines[rand.Intn(len(lines))]
}

func (s *Service) notifyBotActionDM(ctx context.Context, state *game.GameState, text string) {
	if state == nil || strings.TrimSpace(text) == "" {
		return
	}
	if err := s.messenger.SendDM(ctx, state.Game.HostJID, "[bot action] "+text); err == nil {
		log.Printf("bot action dm group=%s host=%s text=%q", state.Game.GroupJID, state.Game.HostJID, text)
	}
}

func verdictWord(guilty bool) string {
	if guilty {
		return "guilty"
	}
	return "not guilty"
}
