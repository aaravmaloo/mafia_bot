package commands

import (
	"context"
	"fmt"
	"log"
	"strings"

	"mafia-bot/game"
	"mafia-bot/models"
)

func (r *Router) routeKill(ctx context.Context, msg InboundMessage, args string) {
	state, player := r.lookupPlayerGame(msg)
	if state == nil || player == nil {
		return
	}
	if state.Phase() != models.PhaseNight {
		r.replyError(ctx, msg, "Kills only work during the night phase.")
		return
	}
	if !player.Alive {
		r.replyError(ctx, msg, "Dead players can't act.")
		return
	}
	if player.Role != models.RoleMafia {
		r.replyError(ctx, msg, fmt.Sprintf("Only mafia can use %s.", formatCommand(r.cfg.Prefix, "kill")))
		return
	}

	target, err := state.ResolveTarget(args, msg.Mentions, true)
	if err != nil {
		r.replyError(ctx, msg, err.Error()+targetHelp(state, player))
		return
	}
	if target.Role == models.RoleMafia {
		r.replyError(ctx, msg, "Mafia can't target their own team.")
		return
	}

	if err = r.service.SubmitKill(ctx, state, player, target); err != nil {
		r.replyError(ctx, msg, err.Error())
	}
}

func (r *Router) routeSave(ctx context.Context, msg InboundMessage, args string) {
	state, player := r.lookupPlayerGame(msg)
	if state == nil || player == nil {
		return
	}
	if state.Phase() != models.PhaseNight {
		r.replyError(ctx, msg, "Saves only work during the night phase.")
		return
	}
	if !player.Alive {
		r.replyError(ctx, msg, "Dead players can't act.")
		return
	}
	if player.Role != models.RoleDoctor {
		r.replyError(ctx, msg, fmt.Sprintf("Only the doctor can use %s.", formatCommand(r.cfg.Prefix, "save")))
		return
	}

	target, err := state.ResolveTarget(args, msg.Mentions, true)
	if err != nil {
		r.replyError(ctx, msg, err.Error()+targetHelp(state, player))
		return
	}

	if err = r.service.SubmitSave(ctx, state, player, target); err != nil {
		r.replyError(ctx, msg, err.Error())
	}
}

func (r *Router) routeInvestigate(ctx context.Context, msg InboundMessage, args string) {
	state, player := r.lookupPlayerGame(msg)
	if state == nil || player == nil {
		return
	}
	if state.Phase() != models.PhaseNight {
		r.replyError(ctx, msg, "Investigations only work during the night phase.")
		return
	}
	if !player.Alive {
		r.replyError(ctx, msg, "Dead players can't act.")
		return
	}
	if player.Role != models.RolePolice {
		r.replyError(ctx, msg, fmt.Sprintf("Only police can use %s.", formatCommand(r.cfg.Prefix, "investigate")))
		return
	}

	target, err := state.ResolveTarget(args, msg.Mentions, true)
	if err != nil {
		r.replyError(ctx, msg, err.Error()+targetHelp(state, player))
		return
	}

	if err = r.service.SubmitInvestigation(ctx, state, player, target); err != nil {
		r.replyError(ctx, msg, err.Error())
	}
}

func (r *Router) routeNominate(ctx context.Context, msg InboundMessage, args string) {
	if !msg.IsGroup {
		return
	}

	state := r.games.Get(msg.ChatJID)
	if state == nil {
		return
	}
	player := state.GetPlayer(msg.SenderJID)
	if player == nil {
		r.replyError(ctx, msg, "You aren't part of this game.")
		return
	}
	if state.Phase() != models.PhaseDay {
		r.replyError(ctx, msg, "Nominations only happen during the day.")
		return
	}
	if !player.Alive {
		r.replyError(ctx, msg, "Dead players can't nominate anyone.")
		return
	}

	target, err := state.ResolveTarget(args, msg.Mentions, true)
	if err != nil {
		r.replyError(ctx, msg, err.Error()+targetHelp(state, player))
		return
	}
	if target.JID.ToNonAD() == player.JID.ToNonAD() {
		r.replyError(ctx, msg, "You can't nominate yourself.")
		return
	}

	if err = r.service.SubmitNomination(ctx, state, player, target); err != nil {
		r.replyError(ctx, msg, err.Error())
	}
}

func (r *Router) routeVerdict(ctx context.Context, msg InboundMessage, guilty bool) {
	if !msg.IsGroup {
		return
	}

	state := r.games.Get(msg.ChatJID)
	if state == nil {
		return
	}
	player := state.GetPlayer(msg.SenderJID)
	if player == nil {
		r.replyError(ctx, msg, "You aren't part of this game.")
		return
	}
	if state.Phase() != models.PhaseVoting {
		r.replyError(ctx, msg, "Verdict votes only happen during the voting phase.")
		return
	}
	if !player.Alive {
		r.replyError(ctx, msg, "Dead players can't vote.")
		return
	}

	if err := r.service.SubmitVerdict(ctx, state, player, guilty); err != nil {
		r.replyError(ctx, msg, err.Error())
	}
}

func (s *Service) SubmitKill(ctx context.Context, state *game.GameState, actor, target *models.Player) error {
	if err := game.ProcessKill(state, game.NormalizeJID(actor.JID), game.NormalizeJID(target.JID)); err != nil {
		return err
	}
	log.Printf("night action group=%s actor=%s role=%s action=kill target=%s", state.Game.GroupJID, actor.Name, actor.Role, target.Name)
	if err := s.messenger.SendDM(ctx, actor.JID, fmt.Sprintf("Kill vote locked on %s.", target.Name)); err != nil {
		return err
	}
	if game.AllNightActionsReceived(state) {
		s.finishNight(context.Background(), state.GroupKey())
	}
	return nil
}

func (s *Service) SubmitSave(ctx context.Context, state *game.GameState, actor, target *models.Player) error {
	if err := game.ProcessSave(state, game.NormalizeJID(actor.JID), game.NormalizeJID(target.JID)); err != nil {
		return err
	}
	log.Printf("night action group=%s actor=%s role=%s action=save target=%s", state.Game.GroupJID, actor.Name, actor.Role, target.Name)
	if err := s.messenger.SendDM(ctx, actor.JID, fmt.Sprintf("Protection locked on %s.", target.Name)); err != nil {
		return err
	}
	if game.AllNightActionsReceived(state) {
		s.finishNight(context.Background(), state.GroupKey())
	}
	return nil
}

func (s *Service) SubmitInvestigation(ctx context.Context, state *game.GameState, actor, target *models.Player) error {
	checked, role, err := game.ProcessInvestigate(state, game.NormalizeJID(actor.JID), game.NormalizeJID(target.JID))
	if err != nil {
		return err
	}
	log.Printf("night action group=%s actor=%s role=%s action=investigate target=%s result=%s", state.Game.GroupJID, actor.Name, actor.Role, checked.Name, role)
	if err = s.messenger.SendDM(ctx, actor.JID, fmt.Sprintf("%s is %s", checked.Name, game.InvestigationResult(role))); err != nil {
		return err
	}
	if game.AllNightActionsReceived(state) {
		s.finishNight(context.Background(), state.GroupKey())
	}
	return nil
}

func (s *Service) SubmitNomination(ctx context.Context, state *game.GameState, actor, target *models.Player) error {
	if err := game.RegisterNomination(state, game.NormalizeJID(actor.JID), game.NormalizeJID(target.JID)); err != nil {
		return err
	}
	log.Printf("nomination group=%s actor=%s target=%s", state.Game.GroupJID, actor.Name, target.Name)

	if err := s.messenger.SendGroup(ctx, state.Game.GroupJID, fmt.Sprintf("%s nominated %s.", actor.Name, target.Name)); err != nil {
		return err
	}

	if state.BeginNominationWindow() {
		log.Printf("nomination window opened group=%s duration=%s", state.Game.GroupJID, s.cfg.NominationDuration)
		if err := s.messenger.SendGroup(ctx, state.Game.GroupJID, fmt.Sprintf("Nomination window is open for %s.", s.cfg.NominationDuration)); err != nil {
			return err
		}
		game.StartTimer(state, s.cfg.NominationDuration, func() {
			s.finishDay(context.Background(), state.GroupKey())
		})
	}

	return nil
}

func (s *Service) SubmitVerdict(ctx context.Context, state *game.GameState, actor *models.Player, guilty bool) error {
	if err := game.CastTrialVote(state, game.NormalizeJID(actor.JID), guilty); err != nil {
		return err
	}
	log.Printf("trial vote group=%s voter=%s guilty=%t", state.Game.GroupJID, actor.Name, guilty)
	if game.AllTrialVotesReceived(state) {
		s.finishVoting(context.Background(), state.GroupKey())
	}
	return nil
}

func (s *Service) applyBundle(ctx context.Context, state *game.GameState, bundle game.PhaseBundle) error {
	game.CancelTimer(state)
	log.Printf("apply phase bundle group=%s phase=%s duration=%s", state.Game.GroupJID, bundle.Phase, bundle.Duration)

	if bundle.GroupText != "" {
		if err := s.messenger.SendGroup(ctx, state.Game.GroupJID, bundle.GroupText); err != nil {
			return err
		}
	}
	for _, dm := range bundle.DMs {
		player := state.GetPlayerByKey(dm.RecipientKey)
		if player == nil || player.IsBot {
			continue
		}
		if err := s.messenger.SendDM(ctx, player.JID, dm.Text); err != nil {
			return err
		}
	}

	s.armTimer(state, bundle)
	s.queueBotActions(state, bundle.Phase)
	return nil
}

func (s *Service) armTimer(state *game.GameState, bundle game.PhaseBundle) {
	switch bundle.Phase {
	case models.PhaseNight:
		game.StartTimer(state, bundle.Duration, func() {
			s.finishNight(context.Background(), state.GroupKey())
		})
	case models.PhaseDay:
		game.StartTimer(state, bundle.Duration, func() {
			s.finishDay(context.Background(), state.GroupKey())
		})
	case models.PhaseTrial:
		game.StartTimer(state, bundle.Duration, func() {
			s.finishTrial(context.Background(), state.GroupKey())
		})
	case models.PhaseVoting:
		game.StartTimer(state, bundle.Duration, func() {
			s.finishVoting(context.Background(), state.GroupKey())
		})
	}
}

func (s *Service) finishNight(ctx context.Context, groupKey string) {
	state := s.games.GetByKey(groupKey)
	if state == nil || state.Phase() != models.PhaseNight {
		return
	}

	resolution := game.ProcessNightEnd(state)
	if resolution.Target != nil {
		log.Printf("night resolved group=%s target=%s saved=%t eliminated=%t votes=%v", state.Game.GroupJID, resolution.Target.Name, resolution.Saved, resolution.Eliminated != nil, resolution.VoteSummary)
	} else {
		log.Printf("night resolved group=%s no target votes=%v", state.Game.GroupJID, resolution.VoteSummary)
	}
	if resolution.Saved && resolution.SuccessfulSaveOn != nil {
		if err := s.messenger.SendDM(ctx, resolution.SuccessfulSaveOn.JID, "You were attacked last night, but the doctor saved you."); err == nil {
			log.Printf("night dm sent group=%s recipient=%s type=saved-player", state.Game.GroupJID, resolution.SuccessfulSaveOn.Name)
		}
		if resolution.SavedBy != nil && !resolution.SavedBy.IsBot {
			if err := s.messenger.SendDM(ctx, resolution.SavedBy.JID, fmt.Sprintf("Your save on %s worked.", resolution.SuccessfulSaveOn.Name)); err == nil {
				log.Printf("night dm sent group=%s recipient=%s type=doctor-save-confirm target=%s", state.Game.GroupJID, resolution.SavedBy.Name, resolution.SuccessfulSaveOn.Name)
			}
		}
	}
	if resolution.Eliminated != nil {
		if winner := game.CheckWin(state); winner != game.WinnerNone {
			log.Printf("win condition reached after night group=%s winner=%s", state.Game.GroupJID, winner)
			_ = s.messenger.SendGroup(ctx, state.Game.GroupJID, game.NightSummaryText(resolution))
			s.endGame(ctx, state, winner)
			return
		}
	}

	log.Printf("phase change group=%s from=%s to=%s", state.Game.GroupJID, models.PhaseNight, models.PhaseDay)
	_ = s.applyBundle(ctx, state, game.StartDay(state, game.NightSummaryText(resolution), s.cfg.DayDuration, s.cfg.Prefix))
}

func (s *Service) finishDay(ctx context.Context, groupKey string) {
	state := s.games.GetByKey(groupKey)
	if state == nil || state.Phase() != models.PhaseDay {
		return
	}

	leader := game.NominationLeader(state)
	if leader == nil {
		log.Printf("day ended group=%s without trial", state.Game.GroupJID)
		_ = s.messenger.SendGroup(ctx, state.Game.GroupJID, "Day ended without a trial. Back to night.")
		_ = s.applyBundle(ctx, state, game.StartNight(state, s.cfg.NightDuration, s.cfg.Prefix))
		return
	}
	log.Printf("trial selected group=%s target=%s", state.Game.GroupJID, leader.Name)

	bundle, err := game.StartTrial(state, game.NormalizeJID(leader.JID), s.cfg.TrialDuration)
	if err == nil {
		_ = s.applyBundle(ctx, state, bundle)
	}
}

func (s *Service) finishTrial(ctx context.Context, groupKey string) {
	state := s.games.GetByKey(groupKey)
	if state == nil || state.Phase() != models.PhaseTrial {
		return
	}

	log.Printf("phase change group=%s from=%s to=%s", state.Game.GroupJID, models.PhaseTrial, models.PhaseVoting)
	_ = s.applyBundle(ctx, state, game.StartVoting(state, s.cfg.VotingDuration, s.cfg.Prefix))
}

func (s *Service) finishVoting(ctx context.Context, groupKey string) {
	state := s.games.GetByKey(groupKey)
	if state == nil || state.Phase() != models.PhaseVoting {
		return
	}

	outcome := game.ResolveTrialVote(state)
	switch {
	case outcome.Target == nil:
		log.Printf("voting ended group=%s with no valid target", state.Game.GroupJID)
		_ = s.applyBundle(ctx, state, game.StartNight(state, s.cfg.NightDuration, s.cfg.Prefix))
	case outcome.Eliminated:
		log.Printf("trial result group=%s target=%s guilty=%d not_guilty=%d eliminated=true role=%s", state.Game.GroupJID, outcome.Target.Name, outcome.Guilty, outcome.NotGuilty, outcome.Target.Role)
		_ = s.messenger.SendGroup(ctx, state.Game.GroupJID, fmt.Sprintf("%s was found guilty and eliminated. They were %s.", outcome.Target.Name, outcome.Target.Role))
		if winner := game.CheckWin(state); winner != game.WinnerNone {
			log.Printf("win condition reached after trial group=%s winner=%s", state.Game.GroupJID, winner)
			s.endGame(ctx, state, winner)
			return
		}
		_ = s.applyBundle(ctx, state, game.StartNight(state, s.cfg.NightDuration, s.cfg.Prefix))
	default:
		log.Printf("trial result group=%s target=%s guilty=%d not_guilty=%d eliminated=false", state.Game.GroupJID, outcome.Target.Name, outcome.Guilty, outcome.NotGuilty)
		_ = s.messenger.SendGroup(ctx, state.Game.GroupJID, fmt.Sprintf("%s was found not guilty. Night falls again.", outcome.Target.Name))
		_ = s.applyBundle(ctx, state, game.StartNight(state, s.cfg.NightDuration, s.cfg.Prefix))
	}
}

func (s *Service) endGame(ctx context.Context, state *game.GameState, winner string) {
	log.Printf("game over group=%s winner=%s", state.Game.GroupJID, winner)
	_ = s.applyBundle(ctx, state, game.EndGame(state, fmt.Sprintf("%s win.", game.WinnerLabel(winner))))
	s.games.Delete(state.Game.GroupJID)
}

func targetHelp(state *game.GameState, actor *models.Player) string {
	if state == nil || actor == nil {
		return ""
	}
	names := state.AvailableTargetNames(actor)
	if len(names) == 0 {
		return ""
	}
	return "\nAvailable names: " + strings.Join(names, ", ")
}
