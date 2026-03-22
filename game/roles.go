package game

import (
	"fmt"
	"math"
	"strings"

	"mafia-bot/models"
)

func IsMafia(role models.Role) bool {
	return role == models.RoleMafia
}

func IsDoctor(role models.Role) bool {
	return role == models.RoleDoctor
}

func IsPolice(role models.Role) bool {
	return role == models.RolePolice
}

func AssignRoles(state *GameState) error {
	state.mu.Lock()
	defer state.mu.Unlock()

	if len(state.joinOrder) == 0 {
		return fmt.Errorf("no players joined")
	}

	deck := buildRoleDeck(len(state.joinOrder))
	state.random.Shuffle(len(deck), func(i, j int) {
		deck[i], deck[j] = deck[j], deck[i]
	})

	for index, key := range state.joinOrder {
		player := state.Players[key]
		player.Role = deck[index]
		player.Alive = true
		player.NightActionDone = false
	}

	return nil
}

func RoleDescription(role models.Role) string {
	switch role {
	case models.RoleMafia:
		return "You work with the mafia. Blend in during the day and coordinate the kill at night."
	case models.RoleDoctor:
		return "Each night you can protect one player. If the mafia picks them, they survive."
	case models.RolePolice:
		return "Each night you can investigate one player and learn whether they are Mafia or Innocent."
	default:
		return "You have no night action. Watch behavior, push discussion, and vote well."
	}
}

func RoleRevealMessages(state *GameState) []DirectMessage {
	state.mu.RLock()
	defer state.mu.RUnlock()

	messages := make([]DirectMessage, 0, len(state.joinOrder))
	mafiaNames := make([]string, 0)
	for _, key := range state.joinOrder {
		player := state.Players[key]
		if player.Role == models.RoleMafia {
			mafiaNames = append(mafiaNames, player.Name)
		}
	}

	for _, key := range state.joinOrder {
		player := state.Players[key]
		text := fmt.Sprintf("━━━━━━━━━━━━━━━━━━━━\n🎭 ROLE: %s\n%s", player.Role, RoleDescription(player.Role))
		if player.Role == models.RoleMafia {
			text = text + "\nYour team: " + mafiaTeamLine(player.Name, mafiaNames)
		}
		if targets := availableNamesForRoleLocked(state, key); len(targets) > 0 {
			text += "\nAvailable names: " + strings.Join(targets, ", ")
		}
		text += "\n━━━━━━━━━━━━━━━━━━━━"
		messages = append(messages, DirectMessage{RecipientKey: key, Text: text})
	}

	return messages
}

func buildRoleDeck(playerCount int) []models.Role {
	mafiaCount := max(1, int(math.Ceil(float64(playerCount)/4.0)))
	deck := make([]models.Role, 0, playerCount)

	for range mafiaCount {
		deck = append(deck, models.RoleMafia)
	}
	deck = append(deck, models.RoleDoctor)
	deck = append(deck, models.RolePolice)

	for len(deck) < playerCount {
		deck = append(deck, models.RoleVillager)
	}

	return deck[:playerCount]
}

func mafiaTeamLine(self string, team []string) string {
	if len(team) <= 1 {
		return "just you"
	}

	others := make([]string, 0, len(team)-1)
	for _, name := range team {
		if name != self {
			others = append(others, name)
		}
	}
	if len(others) == 0 {
		return "just you"
	}
	return strings.Join(others, ", ")
}

func availableNamesForRoleLocked(state *GameState, actorKey string) []string {
	actor := state.Players[actorKey]
	if actor == nil {
		return nil
	}

	names := make([]string, 0, len(state.joinOrder))
	for _, key := range state.joinOrder {
		player := state.Players[key]
		if !player.Alive {
			continue
		}
		switch actor.Role {
		case models.RoleMafia:
			if player.Role == models.RoleMafia {
				continue
			}
		case models.RolePolice:
			if player.JID.ToNonAD() == actor.JID.ToNonAD() {
				continue
			}
		case models.RoleDoctor:
		default:
			continue
		}
		names = append(names, player.Name)
	}
	return names
}
