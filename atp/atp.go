// Package atp provides a security layer over train movement.
// It uses a finite state machine with the following states:
//
// On: The train is turned on and ready to start functioning.
// Methods SetTrain, SetTracks and SetInitConditions can only be
// called in this state. Calling method Start() sets status to Active.
//
// Active: The train actually reads setpoints and attempt to move accordingly.
// If sensors record any warning or alarm, state will be set to Warning or Alarm
// respectively. If Sensors record any alarm, state will be set to Alarm.
// Calling Stop() when the train is moving will cause atp to set status to alarm.
// Calling Stop() when the train is stopped will cause atp to set status to On.
//
// Warning: The train can still function, but short-term action is required.
// If the train does not recover from warnings during X seconds,(to be defined)
// the status will be set Alarm. Examples of warnings are speed overrun,
// acceleration or traction/braking force out of limits, etc.
//
// Alarms: Inmediate action is required to ensure passengers safety. The Atp
// takes over control, ignores incoming setpoints and triggers emergency
// brakes in order to fully stop the train. Once the train stops completely,
// status will be set to On. Examples of alarms are semaphore
// red signal overrun or not receiving incoming setpoints for more than X
// seconds (to be defined)
//
// Shutdown: Train attempts to shut down. If the train is not completely stopped
// status will be set to alarm. Else, Atp will finish execution.
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
	"log/syslog"
	"sync"
	"time"

	"github.com/pkg/errors"
	"github.com/trencat/train/core"
)

type cache struct {
	setpoint       core.Setpoint
	setpointUpdate time.Time
	spLock         sync.RWMutex
	sensors        core.Sensors
	sensorsUpdate  time.Time
	sensLock       sync.RWMutex
	status         Status
}

// Atp implements interfaces.ATP.
type Atp struct {
	core      *core.Core
	setpoint  core.Setpoint
	cache     cache
	state     stateMachine
	nextAlarm time.Time
	start     chan struct{}
	stop      chan struct{}
	kill      chan struct{}
	log       *syslog.Writer
}

func (atp *Atp) activeRoutine() {
	// Update setpoint
	atp.cache.spLock.Lock()
	atp.setpoint = atp.cache.setpoint
	atp.cache.spLock.Unlock()

	atp.onRoutine()
}

func (atp *Atp) alarmRoutine() {
	// Trigger emergency brake
	atp.setpoint = atp.core.EmergencyBrakeSetpoint()

	sensors, err := atp.updateSensors()
	if err != nil {
		atp.state.set(Panic)
		atp.updateCache(atp.state.get(), time.Time{})
		panic(fmt.Sprintf("%+v", err))
	}

	if !Stopped(sensors) {
		return
	}

	atp.state.set(On)
	atp.updateCache(atp.state.get(), time.Time{})
}

// Kill finishes atp abruptly, no matter what's its state.
// Use this method wisely.
func (atp *Atp) Kill() {
	atp.log.Warning("Kill() method called.")
	close(atp.kill)
}

// New initialises an Atp instance.
func New(train core.Train, tracks []core.Track, sensors core.Sensors, log *syslog.Writer) (*Atp, error) {
	// TODO: Validate train, validate track, validate sensors
	co, err := core.New(train, tracks, sensors, log)
	if err != nil {
		return &Atp{}, err
	}

	state, err := newStateMachine(log)
	if err != nil {
		return &Atp{}, err
	}

	atp := Atp{
		core:  &co,
		state: state,
		start: make(chan struct{}),
		stop:  make(chan struct{}),
		kill:  make(chan struct{}),
		log:   log,
	}
	atp.state.set(On)

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

	atp.updateCache(sensors, time.Now())
	atp.updateCache(atp.state.get(), time.Time{})

	// Run
	notify := make(chan struct{})
	go atp.run(notify)
	<-notify // Wait until go routine starts

	log.Info("New ATP initialised")

	return &atp, nil
}

func (atp *Atp) onRoutine() {
	sensors, err := atp.updateSensors()
	if err != nil {
		atp.state.set(Panic)
		atp.updateCache(atp.state.get(), time.Time{})
		panic(fmt.Sprintf("%+v", err))
	}

	if sensors.Warnings.Any() {
		atp.state.set(Warning)
		atp.updateCache(atp.state.get(), time.Now())
	}

	if sensors.Alarms.Any() {
		atp.state.set(Alarm)
		atp.updateCache(atp.state.get(), time.Now())
	}
}

func (atp *Atp) run(notify chan struct{}) {
	// Notify run already started
	close(notify)

loop:
	for {
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
			if finish := atp.shutdownRoutine(); finish {
				atp.state.set(Off)
				atp.updateCache(atp.state.get(), time.Time{})
				continue
			}
		case Off:
			break loop
		}

		select {
		case <-atp.start:
			if atp.state.get() == On {
				atp.state.set(Active)
				atp.updateCache(atp.state.get(), time.Time{})
			}
		case <-atp.stop:
			if atp.state.get() != Shutdown {
				atp.state.set(Shutdown)
				atp.updateCache(atp.state.get(), time.Time{})
			}
		case <-atp.kill:
			break loop
		default:
		}

		// TODO: Remove hardcoded constant
		time.Sleep(time.Duration(200) * time.Millisecond)
	}
}

// Sensors returns current core.Sensors value.
func (atp *Atp) Sensors() core.Sensors {
	atp.cache.sensLock.RLock()
	sensors := atp.cache.sensors
	atp.cache.sensLock.RUnlock()

	return sensors
}

// Set updates current setpoint.
func (atp *Atp) Set(setpoint core.Setpoint) error {
	atp.updateCache(setpoint, time.Now())
	return nil
}

func (atp *Atp) shutdownRoutine() bool {
	sensors, err := atp.updateSensors()
	if err != nil {
		atp.state.set(Panic)
		atp.updateCache(atp.state.get(), time.Time{})
		panic(fmt.Sprintf("%+v", err))
	}

	if !Stopped(sensors) {
		if atp.state.get() != Alarm {
			atp.state.set(Alarm)
			atp.updateCache(atp.state.get(), time.Time{})
		}
		return false
	}

	return true
}

// sinceSensorsUpdate returns the duration since the last time
// Sensors were recorded to cache.
func (atp *Atp) sinceSensorsUpdate(t time.Time) time.Duration {
	atp.cache.sensLock.RLock()
	elapsed := t.Sub(atp.cache.sensorsUpdate)
	atp.cache.sensLock.RUnlock()

	return elapsed
}

// sinceSetpointUpdate returns the duration since the last time
// Setpoint was recorded to cache.
func (atp *Atp) sinceSetpointUpdate(t time.Time) time.Duration {
	atp.cache.sensLock.RLock()
	elapsed := t.Sub(atp.cache.setpointUpdate)
	atp.cache.sensLock.RUnlock()

	return elapsed
}

// Start attempts to set status to Active, thus train
// moves according to Setpoints. Calling this method
// if make no effect if status is different from On.
// Returns error if Start() is called before setting
// a setpoint with Set() method.
func (atp *Atp) Start() error {
	atp.cache.spLock.Lock()
	if atp.cache.setpointUpdate.IsZero() {
		atp.cache.spLock.Unlock()
		err := errors.New("Calling Start without setting Setpoint first")
		atp.log.Warning(fmt.Sprintf("%+v", err))
		return err
	}
	atp.cache.spLock.Unlock()

	atp.start <- struct{}{}
	return nil
}

// Status returns current Atp status
func (atp *Atp) Status() Status {
	atp.cache.sensLock.RLock()
	status := atp.cache.status
	atp.cache.sensLock.RUnlock()

	return status
}

// Stop triggers the stopping routine.
func (atp *Atp) Stop() {
	atp.stop <- struct{}{}
}

// Stopped returns true if the train is completely stopped.
func Stopped(sensors core.Sensors) bool {
	return (sensors.Velocity < 0.01 && sensors.Acceleration < 0.01)
}

func (atp *Atp) updateCache(v interface{}, t time.Time) error {
	switch value := v.(type) {
	case core.Setpoint:
		atp.cache.spLock.Lock()
		atp.cache.setpoint = value
		atp.cache.setpointUpdate = t
		atp.cache.spLock.Unlock()
	case core.Sensors:
		atp.cache.sensLock.Lock()
		atp.cache.sensors = value
		atp.cache.sensorsUpdate = t
		atp.cache.sensLock.Unlock()
	case Status:
		atp.cache.sensLock.Lock()
		atp.cache.status = value
		atp.cache.sensLock.Unlock()
	default:
		err := errors.Errorf("Cannot update buffer with unknown value %+v", v)
		return err
	}

	return nil
}

func (atp *Atp) updateSensors() (core.Sensors, error) {
	now := time.Now()
	elapsed := atp.sinceSensorsUpdate(now)
	setpointElapsed := atp.sinceSetpointUpdate(now)

	sensors, err := atp.core.UpdateSensors(atp.core, atp.setpoint, elapsed, setpointElapsed)
	if err != nil {
		return sensors, err
	}

	// Delete heartbeat alarm if state is On
	heartbeatAlarm := (len(sensors.Alarms.Heartbeat) >= 1)
	state := atp.state.get()
	if heartbeatAlarm && state == On {
		sensors.Alarms.Heartbeat = nil
	}

	atp.updateCache(sensors, now)

	return sensors, nil
}

func (atp *Atp) warningRoutine() {
	atp.activeRoutine()

	// Check status is still Warning.
	if atp.state.get() != Warning {
		return
	}

	sensors := atp.Sensors()
	// Activate/Deactivate next alarm trigger from warnings
	if sensors.Warnings.Any() {
		if atp.nextAlarm.IsZero() {
			atp.nextAlarm = time.Now().Add(time.Duration(5) * time.Second)
		}
	} else {
		if !atp.nextAlarm.IsZero() {
			atp.nextAlarm = time.Time{}
		}

		// Set status before Warning
		if prev := atp.state.prev(); prev == On || prev == Active {
			atp.state.set(prev)
			atp.updateCache(atp.state.get(), time.Time{})
			return
		}
	}

	// Trigger alarm
	if !atp.nextAlarm.IsZero() && time.Since(atp.nextAlarm) > 0 {
		atp.state.set(Alarm)
		atp.updateCache(atp.state.get(), time.Time{})
		atp.nextAlarm = time.Time{}
	}
}
