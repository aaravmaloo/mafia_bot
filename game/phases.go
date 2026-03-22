package game

import (
	"fmt"
	"strings"
	"time"

	"mafia-bot/models"
)

type DirectMessage struct {
	RecipientKey string
	Text         string
}

type PhaseBundle struct {
	Phase     models.Phase
	GroupText string
	DMs       []DirectMessage
	Duration  time.Duration
}

func StartNight(state *GameState, duration time.Duration, prefix string) PhaseBundle {
	state.mu.Lock()
	defer state.mu.Unlock()

	state.Game.Phase = models.PhaseNight
	state.resetNightActionsLocked()
	state.resetDayStateLocked()

	bundle := PhaseBundle{
		Phase:     models.PhaseNight,
		GroupText: "━━━━━━━━━━━━━━━━━━━━\n🌙 NIGHT PHASE\nKeep the group quiet.\nSend night actions in DM.\n━━━━━━━━━━━━━━━━━━━━",
		Duration:  duration,
	}

	mafiaNames := make([]string, 0)
	for _, key := range state.joinOrder {
		player := state.Players[key]
		if player.Alive && player.Role == models.RoleMafia {
			mafiaNames = append(mafiaNames, player.Name)
		}
	}

	for _, key := range state.joinOrder {
		player := state.Players[key]
		if !player.Alive {
			continue
		}

		var text string
		switch player.Role {
		case models.RoleMafia:
			text = fmt.Sprintf("🌙 Night phase\n🔪 Who do you kill?\nCommand: %skill @player", prefix)
			text += "\nYour team: " + mafiaTeamLine(player.Name, mafiaNames)
			if names := availableNamesForRoleLocked(state, key); len(names) > 0 {
				text += "\nAvailable names: " + strings.Join(names, ", ")
			}
		case models.RoleDoctor:
			text = fmt.Sprintf("🌙 Night phase\n💊 Who do you save?\nCommand: %ssave @player", prefix)
			if names := availableNamesForRoleLocked(state, key); len(names) > 0 {
				text += "\nAvailable names: " + strings.Join(names, ", ")
			}
		case models.RolePolice:
			text = fmt.Sprintf("🌙 Night phase\n🔍 Who do you investigate?\nCommand: %sinvestigate @player", prefix)
			if names := availableNamesForRoleLocked(state, key); len(names) > 0 {
				text += "\nAvailable names: " + strings.Join(names, ", ")
			}
		default:
			text = "🌙 Night phase\n😴 Go to sleep... (no night action)"
		}

		bundle.DMs = append(bundle.DMs, DirectMessage{RecipientKey: key, Text: text})
	}

	return bundle
}

func StartDay(state *GameState, opening string, duration time.Duration, prefix string) PhaseBundle {
	state.mu.Lock()
	defer state.mu.Unlock()

	state.Game.Phase = models.PhaseDay
	state.resetDayStateLocked()

	if strings.TrimSpace(opening) == "" {
		opening = fmt.Sprintf("━━━━━━━━━━━━━━━━━━━━\n☀️ DAY PHASE\nDebate, accuse, and nominate.\nCommand: %snominate @player\n━━━━━━━━━━━━━━━━━━━━", prefix)
	}

	return PhaseBundle{
		Phase:     models.PhaseDay,
		GroupText: opening,
		Duration:  duration,
	}
}

func StartTrial(state *GameState, targetKey string, duration time.Duration) (PhaseBundle, error) {
	state.mu.Lock()
	defer state.mu.Unlock()

	target := state.Players[targetKey]
	if target == nil || !target.Alive {
		return PhaseBundle{}, fmt.Errorf("trial target is invalid")
	}

	state.Game.Phase = models.PhaseTrial
	state.TrialTarget = targetKey
	state.TrialVotes = make(map[string]bool)

	bundle := PhaseBundle{
		Phase:     models.PhaseTrial,
		GroupText: fmt.Sprintf("━━━━━━━━━━━━━━━━━━━━\n⚖️ TRIAL\n%s is on trial.\nThey have 60 seconds to defend themselves.\n━━━━━━━━━━━━━━━━━━━━", target.Name),
		Duration:  duration,
		DMs: []DirectMessage{
			{RecipientKey: targetKey, Text: "━━━━━━━━━━━━━━━━━━━━\n⚖️ You are on trial.\nYou have 60 seconds to defend yourself.\n━━━━━━━━━━━━━━━━━━━━"},
		},
	}
	return bundle, nil
}

func StartVoting(state *GameState, duration time.Duration, prefix string) PhaseBundle {
	state.mu.Lock()
	defer state.mu.Unlock()

	target := state.Players[state.TrialTarget]
	name := "the accused"
	if target != nil {
		name = target.Name
	}

	state.Game.Phase = models.PhaseVoting
	state.TrialVotes = make(map[string]bool)

	return PhaseBundle{
		Phase:     models.PhaseVoting,
		GroupText: fmt.Sprintf("━━━━━━━━━━━━━━━━━━━━\n🗳️ VOTING\nDecide %s's fate.\nCommands: %sguilty / %snotguilty\n━━━━━━━━━━━━━━━━━━━━", name, prefix, prefix),
		Duration:  duration,
	}
}

func EndGame(state *GameState, outcome string) PhaseBundle {
	state.mu.Lock()
	defer state.mu.Unlock()

	state.Game.Phase = models.PhaseEnded
	lines := []string{
		"━━━━━━━━━━━━━━━━━━━━",
		fmt.Sprintf("🏆 GAME OVER\n%s", strings.TrimSpace(outcome)),
	}
	for _, key := range state.joinOrder {
		player := state.Players[key]
		status := "alive"
		if !player.Alive {
			status = "dead"
		}
		lines = append(lines, fmt.Sprintf("- %s - %s (%s)", player.Name, player.Role, status))
	}

	return PhaseBundle{
		Phase:     models.PhaseEnded,
		GroupText: strings.Join(append(lines, "━━━━━━━━━━━━━━━━━━━━"), "\n"),
	}
}
