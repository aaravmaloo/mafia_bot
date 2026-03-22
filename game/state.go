package game

import (
	"errors"
	"fmt"
	"math/rand"
	"slices"
	"strings"
	"sync"
	"time"
	"unicode"

	"mafia-bot/models"

	"go.mau.fi/whatsmeow/types"
)

var ErrGameAlreadyExists = errors.New("game already exists in this group")

type Registry struct {
	mu    sync.RWMutex
	games map[string]*GameState
}

type GameState struct {
	mu sync.RWMutex

	Game      models.Game
	Players   map[string]*models.Player
	joinOrder []string

	MafiaVotes   map[string]string
	DoctorSaves  map[string]string
	PoliceChecks map[string]string

	Nominations          map[string]string
	NominationWindowOpen bool
	TrialTarget          string
	TrialVotes           map[string]bool

	timer         *time.Timer
	timerToken    uint64
	phaseDeadline time.Time
	random        *rand.Rand
}

func NewRegistry() *Registry {
	return &Registry{games: make(map[string]*GameState)}
}

func (r *Registry) Create(groupJID, hostJID types.JID) (*GameState, error) {
	r.mu.Lock()
	defer r.mu.Unlock()

	key := NormalizeJID(groupJID)
	if _, exists := r.games[key]; exists {
		return nil, ErrGameAlreadyExists
	}

	state := NewGameState(groupJID, hostJID)
	r.games[key] = state
	return state, nil
}

func (r *Registry) Get(groupJID types.JID) *GameState {
	return r.GetByKey(NormalizeJID(groupJID))
}

func (r *Registry) GetByKey(groupKey string) *GameState {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return r.games[groupKey]
}

func (r *Registry) Delete(groupJID types.JID) {
	r.mu.Lock()
	defer r.mu.Unlock()
	delete(r.games, NormalizeJID(groupJID))
}

func (r *Registry) FindByPlayer(playerJID types.JID) *GameState {
	playerKey := NormalizeJID(playerJID)

	r.mu.RLock()
	defer r.mu.RUnlock()

	for _, state := range r.games {
		if state.HasPlayerKey(playerKey) {
			return state
		}
	}

	return nil
}

func NewGameState(groupJID, hostJID types.JID) *GameState {
	return &GameState{
		Game: models.Game{
			GroupJID: groupJID.ToNonAD(),
			HostJID:  hostJID.ToNonAD(),
			Phase:    models.PhaseLobby,
		},
		Players:      make(map[string]*models.Player),
		MafiaVotes:   make(map[string]string),
		DoctorSaves:  make(map[string]string),
		PoliceChecks: make(map[string]string),
		Nominations:  make(map[string]string),
		TrialVotes:   make(map[string]bool),
		random:       rand.New(rand.NewSource(time.Now().UnixNano())),
	}
}

func NormalizeJID(jid types.JID) string {
	return jid.ToNonAD().String()
}

func (g *GameState) HostKey() string {
	return NormalizeJID(g.Game.HostJID)
}

func (g *GameState) GroupKey() string {
	return NormalizeJID(g.Game.GroupJID)
}

func (g *GameState) Phase() models.Phase {
	g.mu.RLock()
	defer g.mu.RUnlock()
	return g.Game.Phase
}

func (g *GameState) SetPhase(phase models.Phase) {
	g.mu.Lock()
	defer g.mu.Unlock()
	g.Game.Phase = phase
}

func (g *GameState) AddPlayer(jid types.JID, name string) (*models.Player, error) {
	g.mu.Lock()
	defer g.mu.Unlock()

	key := NormalizeJID(jid)
	if existing, ok := g.Players[key]; ok {
		if strings.TrimSpace(name) != "" {
			existing.Name = name
		}
		return existing, nil
	}

	player := &models.Player{
		JID:   jid.ToNonAD(),
		Name:  fallbackName(name, jid),
		Alive: true,
		Phone: jid.User,
		Role:  models.RoleVillager,
	}
	g.Players[key] = player
	g.joinOrder = append(g.joinOrder, key)
	return player, nil
}

func (g *GameState) HasPlayer(jid types.JID) bool {
	return g.HasPlayerKey(NormalizeJID(jid))
}

func (g *GameState) HasPlayerKey(key string) bool {
	g.mu.RLock()
	defer g.mu.RUnlock()
	_, ok := g.Players[key]
	return ok
}

func (g *GameState) GetPlayer(jid types.JID) *models.Player {
	return g.GetPlayerByKey(NormalizeJID(jid))
}

func (g *GameState) GetPlayerByKey(key string) *models.Player {
	g.mu.RLock()
	defer g.mu.RUnlock()
	return g.Players[key]
}

func (g *GameState) PlayerCount() int {
	g.mu.RLock()
	defer g.mu.RUnlock()
	return len(g.Players)
}

func (g *GameState) JoinedPlayers() []*models.Player {
	g.mu.RLock()
	defer g.mu.RUnlock()

	players := make([]*models.Player, 0, len(g.joinOrder))
	for _, key := range g.joinOrder {
		players = append(players, g.Players[key])
	}
	return players
}

func (g *GameState) AlivePlayers() []*models.Player {
	g.mu.RLock()
	defer g.mu.RUnlock()

	alive := make([]*models.Player, 0, len(g.Players))
	for _, key := range g.joinOrder {
		player := g.Players[key]
		if player.Alive {
			alive = append(alive, player)
		}
	}
	return alive
}

func (g *GameState) AlivePlayerKeys() []string {
	g.mu.RLock()
	defer g.mu.RUnlock()

	keys := make([]string, 0, len(g.joinOrder))
	for _, key := range g.joinOrder {
		if g.Players[key].Alive {
			keys = append(keys, key)
		}
	}
	return keys
}

func (g *GameState) AlivePlayersByRole(role models.Role) []*models.Player {
	g.mu.RLock()
	defer g.mu.RUnlock()

	players := make([]*models.Player, 0, len(g.Players))
	for _, key := range g.joinOrder {
		player := g.Players[key]
		if player.Alive && player.Role == role {
			players = append(players, player)
		}
	}
	return players
}

func (g *GameState) PlayerKeysByRole(role models.Role, aliveOnly bool) []string {
	g.mu.RLock()
	defer g.mu.RUnlock()

	keys := make([]string, 0, len(g.Players))
	for _, key := range g.joinOrder {
		player := g.Players[key]
		if player.Role != role {
			continue
		}
		if aliveOnly && !player.Alive {
			continue
		}
		keys = append(keys, key)
	}
	return keys
}

func (g *GameState) AliveCount() int {
	g.mu.RLock()
	defer g.mu.RUnlock()

	total := 0
	for _, player := range g.Players {
		if player.Alive {
			total++
		}
	}
	return total
}

func (g *GameState) MarkEliminated(targetKey string) *models.Player {
	g.mu.Lock()
	defer g.mu.Unlock()

	player := g.Players[targetKey]
	if player == nil {
		return nil
	}

	player.Alive = false
	player.NightActionDone = false
	return player
}

func (g *GameState) ResolveTarget(raw string, mentions []types.JID, aliveOnly bool) (*models.Player, error) {
	g.mu.RLock()
	defer g.mu.RUnlock()

	if len(mentions) > 0 {
		for _, mention := range mentions {
			if player := g.matchKeyLocked(NormalizeJID(mention), aliveOnly); player != nil {
				return player, nil
			}
		}
	}

	query := strings.TrimSpace(strings.TrimPrefix(raw, "@"))
	if query == "" {
		return nil, errors.New("pick a target")
	}

	if digits := digitsOnly(query); digits != "" {
		for _, key := range g.joinOrder {
			player := g.Players[key]
			if aliveOnly && !player.Alive {
				continue
			}
			if player.Phone == digits || strings.HasSuffix(player.Phone, digits) || strings.HasSuffix(player.JID.User, digits) {
				return player, nil
			}
		}
	}

	lowerQuery := strings.ToLower(query)
	var matches []*models.Player
	for _, key := range g.joinOrder {
		player := g.Players[key]
		if aliveOnly && !player.Alive {
			continue
		}
		name := strings.ToLower(strings.TrimSpace(player.Name))
		if name == lowerQuery || strings.Contains(name, lowerQuery) {
			matches = append(matches, player)
		}
	}

	switch len(matches) {
	case 0:
		return nil, fmt.Errorf("couldn't find %q in this game", query)
	case 1:
		return matches[0], nil
	default:
		return nil, fmt.Errorf("%q matches multiple players", query)
	}
}

func (g *GameState) matchKeyLocked(key string, aliveOnly bool) *models.Player {
	player := g.Players[key]
	if player == nil {
		return nil
	}
	if aliveOnly && !player.Alive {
		return nil
	}
	return player
}

func (g *GameState) ResetNightActions() {
	g.mu.Lock()
	defer g.mu.Unlock()
	g.resetNightActionsLocked()
}

func (g *GameState) ResetDayState() {
	g.mu.Lock()
	defer g.mu.Unlock()
	g.resetDayStateLocked()
}

func (g *GameState) resetNightActionsLocked() {
	g.MafiaVotes = make(map[string]string)
	g.DoctorSaves = make(map[string]string)
	g.PoliceChecks = make(map[string]string)
	for _, player := range g.Players {
		player.NightActionDone = false
	}
}

func (g *GameState) resetDayStateLocked() {
	g.Nominations = make(map[string]string)
	g.NominationWindowOpen = false
	g.TrialTarget = ""
	g.TrialVotes = make(map[string]bool)
}

func (g *GameState) NominationCounts() map[string]int {
	g.mu.RLock()
	defer g.mu.RUnlock()

	counts := make(map[string]int)
	for _, target := range g.Nominations {
		counts[target]++
	}
	return counts
}

func (g *GameState) BeginNominationWindow() bool {
	g.mu.Lock()
	defer g.mu.Unlock()

	if g.NominationWindowOpen {
		return false
	}
	g.NominationWindowOpen = true
	return true
}

func (g *GameState) NominationWindowStarted() bool {
	g.mu.RLock()
	defer g.mu.RUnlock()
	return g.NominationWindowOpen
}

func (g *GameState) TeamNames(role models.Role) []string {
	g.mu.RLock()
	defer g.mu.RUnlock()

	names := make([]string, 0, len(g.Players))
	for _, key := range g.joinOrder {
		player := g.Players[key]
		if player.Role == role && player.Alive {
			names = append(names, player.Name)
		}
	}
	slices.Sort(names)
	return names
}

func (g *GameState) RevealLines() []string {
	g.mu.RLock()
	defer g.mu.RUnlock()

	lines := make([]string, 0, len(g.joinOrder))
	for _, key := range g.joinOrder {
		player := g.Players[key]
		status := "alive"
		if !player.Alive {
			status = "dead"
		}
		lines = append(lines, fmt.Sprintf("- %s - %s (%s)", player.Name, player.Role, status))
	}
	return lines
}

func fallbackName(name string, jid types.JID) string {
	name = strings.TrimSpace(name)
	if name != "" {
		return name
	}
	if jid.User != "" {
		return jid.User
	}
	return "Unknown"
}

func digitsOnly(value string) string {
	return strings.Map(func(r rune) rune {
		if unicode.IsDigit(r) {
			return r
		}
		return -1
	}, value)
}
