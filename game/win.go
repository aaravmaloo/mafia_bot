package game

import (
	"strings"

	"mafia-bot/models"
)

const (
	WinnerNone    = ""
	WinnerVillage = "village"
	WinnerMafia   = "mafia"
)

func CheckWin(state *GameState) string {
	state.mu.RLock()
	defer state.mu.RUnlock()

	mafiaAlive := 0
	townAlive := 0
	for _, player := range state.Players {
		if !player.Alive {
			continue
		}
		if player.Role == models.RoleMafia {
			mafiaAlive++
			continue
		}
		townAlive++
	}

	switch {
	case mafiaAlive == 0:
		return WinnerVillage
	case mafiaAlive >= townAlive:
		return WinnerMafia
	default:
		return WinnerNone
	}
}

func WinnerLabel(winner string) string {
	switch winner {
	case WinnerMafia:
		return "Mafia"
	case WinnerVillage:
		return "Village"
	default:
		return strings.Title(winner)
	}
}
