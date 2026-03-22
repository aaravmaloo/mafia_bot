package models

import "go.mau.fi/whatsmeow/types"

type Player struct {
	JID             types.JID
	Name            string
	Role            Role
	Alive           bool
	NightActionDone bool
	Phone           string
	IsBot           bool
}
