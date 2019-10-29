// Package atp provides a security layer over train movement.
// It uses a finite state machine with the following states:
//
// On: The train is turned on and ready to start functioning.
// Calling method Start() sets state to Active.
//
// Active: The train actually reads setpoints and attempt to move accordingly.
// If sensors record any warning or alarm, state will be set to Warning or Alarm
// respectively. Calling Stop() when the train is moving will cause atp to set
// state to alarm. Calling Stop() when the train is stopped will cause atp to
// set state to On.
//
// Warning: The train can still function, but short-term action is required.
// If the train does not recover from warnings during X seconds,(to be defined)
// the state will be set Alarm. Examples of warnings are speed overrun,
// acceleration or traction/braking force out of limits, etc.
//
// Alarms: Inmediate action is required to ensure passengers safety. The Atp
// takes over control, ignores incoming setpoints and triggers emergency
// brakes in order to fully stop the train. Once the train stops completely,
// state will be set to On. Examples of alarms are semaphore
// red signal overrun or not receiving incoming setpoints for more than X
// seconds (to be defined)
//
// Shutdown: Train attempts to shut down. If the train is not completely stopped
// state will be set to alarm. Else, Atp will finish execution.
//
// Off: Indicate train finished execution gracefully.
//
// Panic: Unrecoverable error. Train just explodes (Better not to be inside!).
// Execution calls panic and the train stops working. Examples of panic
// are not being able to read train/tracks/sensors information, or running
// out of tracks.
package atp

import (
	"fmt"
	"time"

	"github.com/pkg/errors"
	log "github.com/trencat/goutils/syslog"
	"github.com/trencat/train/core"
)

// api contains channels for get/set methods
type api struct {
	start       chan chan error
	stop        chan struct{}
	kill        chan struct{}
	notifyOff   chan struct{}
	getSensors  chan chan Sensors
	setSetpoint chan core.Setpoint
	setRoute    chan setRouteRequest
}

// Sensors contains core.Sensors data and ATP state
type Sensors struct {
	Sensors core.Sensors
	State   State
}

type setRouteRequest struct {
	route    []core.Track
	response chan error
}

// Atp implements interfaces.ATP.
type Atp struct {
	core         *core.Core
	userSetpoint core.Setpoint
	setpoint     core.Setpoint
	state        stateMachine
	nextAlarm    time.Time // TODO: Review
	api          api
}

// New initialises an Atp instance.
func New(train core.Train, route []core.Track, sensors core.Sensors) (*Atp, error) {
	// Update setpoint time to prevent Heartbeat alarms
	sensors.Setpoint.Time = time.Now()
	sensors.Time = time.Now()

	// TODO: Validate train, validate track, validate sensors
	co, err := core.New(train, route, sensors)
	if err != nil {
		return &Atp{}, err
	}

	state, err := newStateMachine()
	if err != nil {
		return &Atp{}, err
	}

	atp := Atp{
		core:     &co,
		state:    state,
		setpoint: sensors.Setpoint,
		api: api{
			start:       make(chan chan error),
			stop:        make(chan struct{}),
			kill:        make(chan struct{}),
			notifyOff:   make(chan struct{}),
			getSensors:  make(chan chan Sensors),
			setSetpoint: make(chan core.Setpoint),
			setRoute:    make(chan setRouteRequest),
		},
	}

	if sensors.Warnings.Any() {
		if err = atp.state.set(Warning); err != nil {
			return &atp, err
		}
	}
	if sensors.Alarms.Any() {
		if err = atp.state.set(Alarm); err != nil {
			return &atp, err
		}
	}

	// Run
	notify := make(chan struct{})
	go atp.run(notify)
	<-notify // Wait until go routine starts

	log.Info("New ATP initialised")

	return &atp, nil
}

// Sensors returns current core.Sensors and state value.
func (atp *Atp) Sensors() Sensors {
	select {
	case <-atp.api.notifyOff:
		// atp has finished running
		return Sensors{Sensors: atp.core.Sensors(), State: atp.state.get()}
	default:
		ch := make(chan Sensors)
		defer close(ch)

		atp.api.getSensors <- ch
		return <-ch
	}
}

// SetSetpoint updates current setpoint. Calling this method when
// state is Off takes no effect.
func (atp *Atp) SetSetpoint(setpoint core.Setpoint) {
	select {
	case <-atp.api.notifyOff:
		return
	default:
		setpoint.Time = time.Now()
		atp.api.setSetpoint <- setpoint
	}
}

// SetRoute introduces the route that the ATP must follow. Check
// core.SetRoute for more details. Calling this method when state
// is Off takes no effect and a nil is returned.
func (atp *Atp) SetRoute(route []core.Track) error {
	select {
	case <-atp.api.notifyOff:
		// atp has finished running
		return nil
	default:
		errch := make(chan error)
		defer close(errch)

		atp.api.setRoute <- setRouteRequest{
			route:    route,
			response: errch,
		}
		return <-errch
	}
}

// run starts the atp closed loop algorithm. It is divided in three steps:
// the operations step, where Sensors and status values are updated,
// get/set step, where get and set queries are performed and signal step,
// where signals are listened and processed. This three-steps implementation
// avoid the use of locks on common data, since there no two threads accessing
// the same data concurrently.
func (atp *Atp) run(notify chan struct{}) {
	// Notify run already started
	close(notify)

loop:
	for {
		// Operations
		switch atp.state.get() {
		case On:
			atp.onRoutine()
		case Active:
			atp.activeRoutine()
		case Warning:
			atp.warningRoutine()
		case Alarm:
			atp.alarmRoutine()
		case Shutdown:
			if done := atp.shutdownRoutine(); done {
				atp.state.set(Off)
				continue
			}
		case Off:
			atp.offRoutine()
			break loop
		}

		// API Getters and setters
		atp.getRoutine()
		atp.setRoutine()

		// API start/stop/kill signals
		atp.signalsRoutine()

		// TODO: Remove hardcoded constant
		time.Sleep(time.Duration(200) * time.Millisecond)
	}
}

func (atp *Atp) getRoutine() {
	// Get sensors
	select {
	case ch := <-atp.api.getSensors:
		ch <- atp.getSensors()
	default:
		break
	}
}

func (atp *Atp) getSensors() Sensors {
	return Sensors{Sensors: atp.core.Sensors(), State: atp.state.get()}
}

func (atp *Atp) setRoutine() {
	// Set user setpoint
	select {
	case setpoint := <-atp.api.setSetpoint:
		atp.userSetpoint = setpoint
	default:
		break
	}

	// Set route
	select {
	case request := <-atp.api.setRoute:
		if err := atp.core.SetRoute(request.route); err != nil {
			request.response <- err
		}
	default:
		break
	}
}

func (atp *Atp) signalsRoutine() {
	select {
	case ch := <-atp.api.start:
		ch <- atp.startSignalRoutine()
	case <-atp.api.stop:
		state := atp.state.get()
		if state != Shutdown {
			atp.state.set(Shutdown)
		}
	case <-atp.api.kill:
		atp.state.set(Off)
	default:
	}
}

func (atp *Atp) onRoutine() {
	sensors, err := atp.updateSensors()
	if err != nil {
		atp.state.set(Panic)
		panic(fmt.Sprintf("%+v", err))
	}

	if sensors.Warnings.Any() {
		atp.state.set(Warning)
	}

	if sensors.Alarms.Any() {
		atp.state.set(Alarm)
	}
}

func (atp *Atp) activeRoutine() {
	atp.setpoint = atp.userSetpoint
	atp.onRoutine()
}

func (atp *Atp) warningRoutine() {
	atp.activeRoutine()

	// Check state is still Warning.
	if atp.state.get() != Warning {
		return
	}

	sensors := atp.core.Sensors()
	// Activate/Deactivate next alarm trigger from warnings
	if sensors.Warnings.Any() {
		if atp.nextAlarm.IsZero() {
			atp.nextAlarm = time.Now().Add(time.Duration(5) * time.Second)
		}
	} else {
		if !atp.nextAlarm.IsZero() {
			atp.nextAlarm = time.Time{}
		}

		// Set state before Warning
		if prev := atp.state.prev(); prev == On || prev == Active {
			atp.state.set(prev)
			return
		}
	}

	// Trigger alarm
	if !atp.nextAlarm.IsZero() && time.Since(atp.nextAlarm) > 0 {
		atp.state.set(Alarm)
		atp.nextAlarm = time.Time{}
	}
}

func (atp *Atp) alarmRoutine() {
	// Trigger emergency brake
	atp.setpoint = atp.core.EmergencyBrakeSetpoint()

	sensors, err := atp.updateSensors()
	if err != nil {
		atp.state.set(Panic)
		panic(fmt.Sprintf("%+v", err))
	}

	if !Stopped(sensors) {
		return
	}

	atp.state.set(On)
}

func (atp *Atp) shutdownRoutine() bool {
	sensors, err := atp.updateSensors()
	if err != nil {
		atp.state.set(Panic)
		panic(fmt.Sprintf("%+v", err))
	}

	if !Stopped(sensors) {
		// Attempt to shutdown train while running
		if atp.state.get() != Alarm {
			atp.state.set(Alarm)
		}
		return false
	}

	return true
}

func (atp *Atp) offRoutine() {
	close(atp.api.notifyOff)
	close(atp.api.start)
	close(atp.api.stop)
	close(atp.api.kill)
	close(atp.api.setSetpoint)
	close(atp.api.setRoute)
	close(atp.api.getSensors)

	// Empty api channels
	for range atp.api.start {
	}
	for range atp.api.stop {
	}
	for range atp.api.kill {
	}
	for range atp.api.setSetpoint {
	}
	for range atp.api.setRoute {
	}
	for ch := range atp.api.getSensors {
		ch <- atp.getSensors()
	}
}

func (atp *Atp) startSignalRoutine() error {
	// Check state is On and setpoint is set.
	if atp.state.get() != On {
		err := errors.New("Calling Start with status not On")
		log.Warning(fmt.Sprintf("%+v", err))
		return err
	}
	if atp.userSetpoint.Time.IsZero() {
		err := errors.New("Calling Start without setting Setpoint first")
		log.Warning(fmt.Sprintf("%+v", err))
		return err
	}
	atp.state.set(Active)
	return nil
}

// Start attempts to set state to Active, thus train
// moves according to Setpoints. Calling this method
// has no effect if state is different from On.
// Calling this method has no effect if no setpoint
// has been set yet. Calling this method when state
// is Off takes no effect and a nil is returned.
func (atp *Atp) Start() error {
	select {
	case <-atp.api.notifyOff:
		// atp has finished running
		return nil
	default:
		errch := make(chan error)
		defer close(errch)

		atp.api.start <- errch
		return <-errch
	}

}

// Stop triggers the stopping routine. Calling this
// method when state is Off takes no effect.
func (atp *Atp) Stop() {
	select {
	case <-atp.api.notifyOff:
		// atp has finished running
		return
	default:
		atp.api.stop <- struct{}{}
	}
}

// Kill finishes atp abruptly, no matter what's its state.
// Use this method wisely. Calling this method when state
// is Off takes no effect.
func (atp *Atp) Kill() {
	log.Warning("Kill() method called")
	select {
	case <-atp.api.notifyOff:
		// atp has finished running
		return
	default:
		atp.api.kill <- struct{}{}
	}
}

// Stopped returns true if the train is completely stopped.
func Stopped(sensors core.Sensors) bool {
	return (sensors.Velocity < 0.01 && sensors.Acceleration < 0.01)
}

func (atp *Atp) updateSensors() (core.Sensors, error) {
	sensors, err := atp.core.UpdateSensors(atp.setpoint, time.Now())
	if err != nil {
		return sensors, err
	}

	// Delete heartbeat alarm if state is On
	heartbeatAlarm := (len(sensors.Alarms.Heartbeat) >= 1)
	state := atp.state.get()
	if heartbeatAlarm && state == On {
		sensors.Alarms.Heartbeat = nil
	}

	return sensors, nil
}
