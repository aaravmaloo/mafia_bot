package models

type Role string

const (
	RoleMafia    Role = "Mafia"
	RoleDoctor   Role = "Doctor"
	RolePolice   Role = "Police"
	RoleVillager Role = "Villager"
)
