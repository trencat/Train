package atp

import (
	"fmt"

	"github.com/pkg/errors"
	log "github.com/trencat/goutils/syslog"
)

// State represents a state machine state
type State int8

// ATP state machine constants
const (
	Init     State = 0 // For internal use only
	On       State = 10
	Active   State = 20
	Warning  State = 30
	Alarm    State = 40
	Panic    State = 50
	Shutdown State = 60
	Off      State = 70
)

// statemachine is not safe! Locks must be implemented somewhere else
type stateMachine struct {
	state     State
	prevState State
}

// newState declares and initialises a state instance.
func newStateMachine() (stateMachine, error) {
	log.Info("New state machine initialised")
	log.Info(fmt.Sprintf("State set to %d", On))
	return stateMachine{
		state:     On,
		prevState: Init,
	}, nil
}

func (sm *stateMachine) canSet(to State) bool {
	from := sm.state

	return (from == to) ||
		(from == On && to == Active) ||
		(from == Active && to == On) ||
		(from == On && to == Warning) ||
		(from == Active && to == Warning) ||
		(from == Warning && to == Active) ||
		(to == Alarm) ||
		(to == Panic) ||
		(to == Shutdown) ||
		(from == Alarm && to == On) ||
		(to == Off)
}

func (sm *stateMachine) set(to State) error {
	from := sm.state

	if from == to {
		return nil
	}

	if !sm.canSet(to) {
		err := errors.Errorf("Attempt to set state to %d from state %d.", to, from)
		log.Warning(fmt.Sprintf("%+v", err))
		return err
	}

	sm.prevState = sm.state
	sm.state = to
	log.Info(fmt.Sprintf("State set to %d", to))
	return nil
}

func (sm stateMachine) get() State {
	return sm.state
}

func (sm stateMachine) prev() State {
	return sm.prevState
}
