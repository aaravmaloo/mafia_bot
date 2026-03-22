package models

import "go.mau.fi/whatsmeow/types"

type Phase string

const (
	PhaseLobby  Phase = "LOBBY"
	PhaseNight  Phase = "NIGHT"
	PhaseDay    Phase = "DAY"
	PhaseTrial  Phase = "TRIAL"
	PhaseVoting Phase = "VOTING"
	PhaseEnded  Phase = "ENDED"
)

type Game struct {
	GroupJID types.JID
	HostJID  types.JID
	Phase    Phase
}
