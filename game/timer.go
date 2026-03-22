package game

import "time"

func StartTimer(state *GameState, duration time.Duration, onFire func()) {
	if duration <= 0 {
		CancelTimer(state)
		return
	}

	state.mu.Lock()
	state.timerToken++
	token := state.timerToken
	if state.timer != nil {
		state.timer.Stop()
	}
	timer := time.NewTimer(duration)
	state.timer = timer
	state.phaseDeadline = time.Now().Add(duration)
	state.mu.Unlock()

	go func() {
		<-timer.C

		state.mu.Lock()
		stale := token != state.timerToken
		if !stale {
			state.timer = nil
			state.phaseDeadline = time.Time{}
		}
		state.mu.Unlock()

		if stale {
			return
		}
		onFire()
	}()
}

func CancelTimer(state *GameState) {
	state.mu.Lock()
	defer state.mu.Unlock()

	state.timerToken++
	state.phaseDeadline = time.Time{}
	if state.timer != nil {
		state.timer.Stop()
		state.timer = nil
	}
}

func PhaseDeadline(state *GameState) time.Time {
	state.mu.RLock()
	defer state.mu.RUnlock()
	return state.phaseDeadline
}
