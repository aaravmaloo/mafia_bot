package game

import (
	"errors"
	"fmt"
	"slices"

	"mafia-bot/models"
)

type NightResolution struct {
	Target           *models.Player
	Saved            bool
	SavedBy          *models.Player
	Eliminated       *models.Player
	VoteSummary      map[string]int
	SuccessfulSaveOn *models.Player
}

type TrialOutcome struct {
	Target     *models.Player
	Guilty     int
	NotGuilty  int
	Eliminated bool
}

func ProcessKill(state *GameState, actorKey, targetKey string) error {
	state.mu.Lock()
	defer state.mu.Unlock()

	actor := state.Players[actorKey]
	target := state.Players[targetKey]
	if actor == nil || target == nil {
		return errors.New("invalid actor or target")
	}
	if state.Game.Phase != models.PhaseNight {
		return errors.New("kills only work at night")
	}
	if !actor.Alive || !target.Alive {
		return errors.New("dead players can't use night actions")
	}
	if actor.Role != models.RoleMafia {
		return errors.New("only mafia can kill")
	}
	if target.Role == models.RoleMafia {
		return errors.New("mafia can't target their own team")
	}

	state.MafiaVotes[actorKey] = targetKey
	actor.NightActionDone = true
	return nil
}

func ProcessSave(state *GameState, actorKey, targetKey string) error {
	state.mu.Lock()
	defer state.mu.Unlock()

	actor := state.Players[actorKey]
	target := state.Players[targetKey]
	if actor == nil || target == nil {
		return errors.New("invalid actor or target")
	}
	if state.Game.Phase != models.PhaseNight {
		return errors.New("saves only work at night")
	}
	if !actor.Alive || !target.Alive {
		return errors.New("dead players can't use night actions")
	}
	if actor.Role != models.RoleDoctor {
		return errors.New("only the doctor can save")
	}

	state.DoctorSaves[actorKey] = targetKey
	actor.NightActionDone = true
	return nil
}

func ProcessInvestigate(state *GameState, actorKey, targetKey string) (*models.Player, models.Role, error) {
	state.mu.Lock()
	defer state.mu.Unlock()

	actor := state.Players[actorKey]
	target := state.Players[targetKey]
	if actor == nil || target == nil {
		return nil, "", errors.New("invalid actor or target")
	}
	if state.Game.Phase != models.PhaseNight {
		return nil, "", errors.New("investigations only work at night")
	}
	if !actor.Alive || !target.Alive {
		return nil, "", errors.New("dead players can't use night actions")
	}
	if actor.Role != models.RolePolice {
		return nil, "", errors.New("only police can investigate")
	}

	state.PoliceChecks[actorKey] = targetKey
	actor.NightActionDone = true
	return target, target.Role, nil
}

func AllNightActionsReceived(state *GameState) bool {
	state.mu.RLock()
	defer state.mu.RUnlock()

	for _, key := range state.joinOrder {
		player := state.Players[key]
		if !player.Alive {
			continue
		}
		switch player.Role {
		case models.RoleMafia, models.RoleDoctor, models.RolePolice:
			if !player.NightActionDone {
				return false
			}
		}
	}

	return true
}

func ProcessNightEnd(state *GameState) NightResolution {
	state.mu.Lock()
	defer state.mu.Unlock()

	summary := make(map[string]int)
	for _, targetKey := range state.MafiaVotes {
		summary[targetKey]++
	}

	var (
		targetKey string
		topVotes  int
		tied      []string
	)

	for key, votes := range summary {
		switch {
		case votes > topVotes:
			topVotes = votes
			tied = []string{key}
		case votes == topVotes:
			tied = append(tied, key)
		}
	}
	if len(tied) > 0 {
		slices.Sort(tied)
		targetKey = tied[state.random.Intn(len(tied))]
	}

	var target *models.Player
	if targetKey != "" {
		target = state.Players[targetKey]
	}

	saved := false
	var savedBy *models.Player
	if targetKey != "" {
		for doctorKey, savedKey := range state.DoctorSaves {
			if savedKey == targetKey {
				saved = true
				savedBy = state.Players[doctorKey]
				break
			}
		}
	}

	var eliminated *models.Player
	if target != nil && !saved {
		target.Alive = false
		target.NightActionDone = false
		eliminated = target
	}

	state.resetNightActionsLocked()
	state.resetDayStateLocked()

	return NightResolution{
		Target:           target,
		Saved:            saved,
		SavedBy:          savedBy,
		Eliminated:       eliminated,
		VoteSummary:      summary,
		SuccessfulSaveOn: target,
	}
}

func RegisterNomination(state *GameState, nominatorKey, targetKey string) error {
	state.mu.Lock()
	defer state.mu.Unlock()

	nominator := state.Players[nominatorKey]
	target := state.Players[targetKey]
	if nominator == nil || target == nil {
		return errors.New("invalid nominator or target")
	}
	if state.Game.Phase != models.PhaseDay {
		return errors.New("nominations only work during the day")
	}
	if !nominator.Alive || !target.Alive {
		return errors.New("only alive players take part in nominations")
	}

	state.Nominations[nominatorKey] = targetKey
	return nil
}

func NominationLeader(state *GameState) *models.Player {
	state.mu.Lock()
	defer state.mu.Unlock()
	return nominationLeaderLocked(state)
}

func CastTrialVote(state *GameState, voterKey string, guilty bool) error {
	state.mu.Lock()
	defer state.mu.Unlock()

	voter := state.Players[voterKey]
	if voter == nil {
		return errors.New("unknown voter")
	}
	if state.Game.Phase != models.PhaseVoting {
		return errors.New("trial votes only work during voting")
	}
	if !voter.Alive {
		return errors.New("dead players can't vote")
	}

	state.TrialVotes[voterKey] = guilty
	return nil
}

func AllTrialVotesReceived(state *GameState) bool {
	state.mu.RLock()
	defer state.mu.RUnlock()

	for _, key := range state.joinOrder {
		player := state.Players[key]
		if player.Alive {
			if _, voted := state.TrialVotes[key]; !voted {
				return false
			}
		}
	}
	return true
}

func ResolveTrialVote(state *GameState) TrialOutcome {
	state.mu.Lock()
	defer state.mu.Unlock()

	target := state.Players[state.TrialTarget]
	outcome := TrialOutcome{Target: target}
	for _, guilty := range state.TrialVotes {
		if guilty {
			outcome.Guilty++
		} else {
			outcome.NotGuilty++
		}
	}

	if target != nil && target.Alive && outcome.Guilty > outcome.NotGuilty {
		target.Alive = false
		target.NightActionDone = false
		outcome.Eliminated = true
	}

	state.resetDayStateLocked()
	return outcome
}

func nominationLeaderLocked(state *GameState) *models.Player {
	if len(state.Nominations) == 0 {
		return nil
	}

	counts := make(map[string]int)
	bestScore := 0
	best := make([]string, 0)
	for _, targetKey := range state.Nominations {
		counts[targetKey]++
		score := counts[targetKey]
		switch {
		case score > bestScore:
			bestScore = score
			best = []string{targetKey}
		case score == bestScore:
			best = append(best, targetKey)
		}
	}

	if len(best) == 0 {
		return nil
	}
	slices.Sort(best)
	winner := best[state.random.Intn(len(best))]
	return state.Players[winner]
}

func InvestigationResult(role models.Role) string {
	if role == models.RoleMafia {
		return "MAFIA 🔴"
	}
	return "INNOCENT 🟢"
}

func NightSummaryText(resolution NightResolution) string {
	switch {
	case resolution.Target == nil:
		return "☀️ Morning. Nobody was targeted last night."
	case resolution.Saved:
		return "☀️ Nobody died last night 👀"
	default:
		return fmt.Sprintf("☀️ %s was killed 💀", resolution.Target.Name)
	}
}
