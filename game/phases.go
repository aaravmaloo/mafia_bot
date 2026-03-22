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

func StartNight(state *GameState, duration time.Duration) PhaseBundle {
	state.mu.Lock()
	defer state.mu.Unlock()

	state.Game.Phase = models.PhaseNight
	state.resetNightActionsLocked()
	state.resetDayStateLocked()

	bundle := PhaseBundle{
		Phase:     models.PhaseNight,
		GroupText: "🌙 Night phase has started. Keep the group quiet and send night actions in DM.",
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
			text = "🔪 Night phase! Who do you kill?\n!kill @player"
			text += "\nYour team: " + mafiaTeamLine(player.Name, mafiaNames)
		case models.RoleDoctor:
			text = "💊 Night phase! Who do you save?\n!save @player"
		case models.RolePolice:
			text = "🔍 Night phase! Who do you investigate?\n!investigate @player"
		default:
			text = "😴 Go to sleep... (no night action)"
		}

		bundle.DMs = append(bundle.DMs, DirectMessage{RecipientKey: key, Text: text})
	}

	return bundle
}

func StartDay(state *GameState, opening string, duration time.Duration) PhaseBundle {
	state.mu.Lock()
	defer state.mu.Unlock()

	state.Game.Phase = models.PhaseDay
	state.resetDayStateLocked()

	if strings.TrimSpace(opening) == "" {
		opening = "☀️ Day phase has started. Debate, accuse, and nominate with !nominate @player."
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
		GroupText: fmt.Sprintf("⚖️ %s is on trial. They have 60 seconds to defend themselves.", target.Name),
		Duration:  duration,
		DMs: []DirectMessage{
			{RecipientKey: targetKey, Text: "⚖️ You are on trial! 60 seconds to defend yourself."},
		},
	}
	return bundle, nil
}

func StartVoting(state *GameState, duration time.Duration) PhaseBundle {
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
		GroupText: fmt.Sprintf("🗳️ Voting time. Decide %s's fate with !guilty or !notguilty.", name),
		Duration:  duration,
	}
}

func EndGame(state *GameState, outcome string) PhaseBundle {
	state.mu.Lock()
	defer state.mu.Unlock()

	state.Game.Phase = models.PhaseEnded
	lines := []string{fmt.Sprintf("🏆 Game over! %s", strings.TrimSpace(outcome))}
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
		GroupText: strings.Join(lines, "\n"),
	}
}
