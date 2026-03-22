package game

import (
	"fmt"

	"mafia-bot/models"
)

var botNames = []string{
	"Rook",
	"Cipher",
	"Miso",
	"Nova",
	"Echo",
	"Bluff",
	"Vanta",
	"Pixel",
	"Rune",
	"Quartz",
	"Jinx",
	"Comet",
}

func FillBotSeats(state *GameState, minimumPlayers int) []*models.Player {
	missing := minimumPlayers - state.PlayerCount()
	if missing <= 0 {
		return nil
	}

	added := make([]*models.Player, 0, missing)
	start := state.BotPlayerCount()
	for i := 0; i < missing; i++ {
		name := botNames[(start+i)%len(botNames)]
		if start+i >= len(botNames) {
			name = fmt.Sprintf("%s %d", name, start+i+1)
		}
		added = append(added, state.AddBot(name))
	}

	return added
}
