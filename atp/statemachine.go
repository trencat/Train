package atp

import (
	"fmt"
	"log/syslog"

	"github.com/pkg/errors"
)

// Status represents a state machine status
type Status int8

const (
	Init     Status = 0 // For internal use only
	On       Status = 10
	Active   Status = 20
	Warning  Status = 30
	Alarm    Status = 40
	Panic    Status = 50
	Shutdown Status = 60
	Off      Status = 70
)

// statemachine is not safe! Locks must be implemented somewhere else
type stateMachine struct {
	status     Status
	prevStatus Status
	log        *syslog.Writer
}

// newState declares and initialises a state instance.
func newStateMachine(log *syslog.Writer) (stateMachine, error) {
	if log == nil {
		err := errors.New("Attempt to declare a new state machine. Log not provided (nil)")
		fmt.Printf("%+v", err)
		return stateMachine{}, err
	}

	log.Info("New state machine initialised")
	return stateMachine{
		status:     On,
		prevStatus: Init,
		log:        log,
	}, nil
}

func (sm *stateMachine) canSet(to Status) bool {
	from := sm.status

	return (from == to) ||
		(from == On && to == Active) ||
		(from == Active && to == On) ||
		(from == On && to == Warning) ||
		(from == Active && to == Warning) ||
		(from == Warning && to == Active) ||
		(to == Alarm) ||
		(to == Shutdown) ||
		(from == Alarm && to == On) ||
		(from == Shutdown && to == Off)
}

func (sm *stateMachine) set(to Status) error {
	from := sm.status

	if from == to {
		return nil
	}

	if !sm.canSet(to) {
		err := errors.Errorf("Attempt to set status to %d from status %d.", to, from)
		sm.log.Warning(fmt.Sprintf("%+v", err))
		return err
	}

	sm.prevStatus = sm.status
	sm.status = to
	sm.log.Info(fmt.Sprintf("Status set to %d", to))
	return nil
}

func (sm stateMachine) get() Status {
	return sm.status
}

func (sm stateMachine) prev() Status {
	return sm.prevStatus
}
