// Package core provides a simple (yet complete) implementation
// of train movement.
package core

import (
	"fmt"
	"math"
	"time"

	"github.com/pkg/errors"
	log "github.com/trencat/goutils/syslog"
)

const gravity float64 = 9.80665

// Core collects essential information for train automation and
// implements train movement. Implements interfaces.Core.
type Core struct {
	train   Train
	tracks  map[int]Track
	route   []int
	sensors Sensors
}

// Track specifications. Implements interfaces.Track
type Track struct {
	ID          int
	Length      float64
	MaxVelocity float64
	Slope       float64
	BendRadius  float64
	Tunnel      bool
}

// Train specifications. Implements interfaces.Train
type Train struct {
	ID            int
	Mass          float64
	MassFactor    float64
	Length        float64
	MaxTraction   float64
	MaxBrake      float64
	MaxVelocity   float64
	ResistanceLin float64
	ResistanceQua float64
}

type Setpoint struct {
	Value float64
	Time  time.Time
}

// Sensors contains dynamic data collected by train's sensors.
// All values are expressed in the International System of Units.
type Sensors struct {
	Time          time.Time
	Setpoint      Setpoint
	Position      float64 // Relative to the beginning of the track
	Velocity      float64
	Acceleration  float64
	TractionForce float64
	BrakingForce  float64
	TractionPower float64
	BrakingPower  float64
	Mass          float64
	TrackID       int
	RelPosition   float64 // Relative to the current track
	Slope         float64
	BendRadius    float64
	Tunnel        bool
	Resistance    float64 // Basic + line resistance
	BasicRes      float64
	SlopeRes      float64
	CurveRes      float64
	TunnelRes     float64
	LineRes       float64 // slope + curve + tunnel resistance
	NumPassengers int
	Warnings      Warnings
	Alarms        Warnings
}

// New initialises a Core instance.
func New(train Train, route []Track, sensors Sensors) (Core, error) {
	log.Info("New Core initialised")
	core := Core{
		train:   train,
		tracks:  make(map[int]Track),
		sensors: sensors,
	}

	if err := core.addRoute(route); err != nil {
		return Core{}, err
	}

	return core, nil
}

// addRoute adds new tracks to Core memory. Already existing tracks
// will be overwritten. An error is returned if any track's prevID or nextID
// are inconsistent. In case of error, no tracks will be added.
func (c *Core) addRoute(route []Track) error {
	//TODO: Validate tracks

	// Add tracks
	newRoute := make([]int, len(route))
	for i, track := range route {
		c.tracks[track.ID] = track
		newRoute[i] = route[i].ID
	}

	// Set route
	c.route = newRoute

	return nil
}

// getRoute returns the Track at the given position in the route slice.
func (c *Core) getRoute(index int) (Track, error) {
	if index >= len(c.route) || index < 0 {
		err := errors.Errorf("Attempt to getRoute. Position %d out of bounds", index)
		log.Warning(fmt.Sprintf("%+v", err))
		return Track{}, err
	}

	trackID := c.route[index]
	track, exists := c.tracks[trackID]
	if !exists {
		err := errors.Errorf("Attempt to getRoute. At position %d, Track with ID %d does not exist", index, trackID)
		log.Warning(fmt.Sprintf("%+v", err))
		return Track{}, err
	}

	return track, nil
}

// popRoute deletes the element c.route[0]. This method has no effect
// if c.route is not set.
func (c *Core) popRoute() {
	if len(c.route) == 0 {
		return
	}
	c.route = c.route[1:len(c.route)]
}

// Sensors return current sensors values
func (c *Core) Sensors() Sensors {
	return c.sensors
}

// SetRoute establishes the route that the train must follow.
// The route is an ordered slice of Tracks, being the track at position 0
// the train's current Track. This method overwrites the current route (if any).
// An error is returned if route[0] trackID does not match with current
// track that the train is running. An error is returned if any tracks' PrevID and NextID
// are inconsistent. In case of error, the route is not set.
func (c *Core) SetRoute(route []Track) error {

	if len(c.route) > 0 {
		currentTrack, err := c.getRoute(0)
		if err != nil {
			return err
		}
		if route[0].ID != currentTrack.ID {
			err = errors.New("blabla")
			//log error
			return err
		}
	}

	if err := c.addRoute(route); err != nil {
		return err
	}

	return nil
}

// UpdateSensors is a wrapper around core.UpdateSensorsAcceleration. In the future,
// this method will choose between more than one UpdateSensors imlementations.
func (c *Core) UpdateSensors(sp Setpoint, until time.Time) (Sensors, error) {
	return c.UpdateSensorsAcceleration(sp, until)
}

// UpdateSensorsAcceleration updates real time data until a given time.
// Setpoint argument refers to acceleration.
func (c *Core) UpdateSensorsAcceleration(sp Setpoint, until time.Time) (Sensors, error) {
	//TODO: Remove hardcoded constants
	//TODO: Watch out, many log errors may happen.

	prev := &c.sensors
	new := Sensors{}
	train := &c.train

	warnings := Warnings{}
	alarms := Warnings{}

	// Track
	track, err := c.getRoute(0)
	if err != nil {
		return Sensors{}, err
	}

	// Update track, trackIndex
	beginNewTrack := (prev.RelPosition > track.Length)
	if beginNewTrack {
		c.popRoute()
		track, err = c.getRoute(0)
		if err != nil {
			return Sensors{}, err
		}
	}

	// TrackID
	new.TrackID = track.ID

	// Time
	deltaSec := until.Sub(prev.Time).Seconds()
	new.Time = until

	// Number of passengers
	new.NumPassengers = prev.NumPassengers

	// Mass (add average mass for each passenger)
	// TODO: Remove hardcoded mass 70
	new.Mass = c.train.Mass + float64(new.NumPassengers)*70

	// Setpoint
	new.Setpoint = sp

	// Velocity
	new.Velocity = math.Max(0.0, prev.Velocity+deltaSec*prev.Acceleration)
	if new.Velocity > c.train.MaxVelocity {
		log.Warning(fmt.Sprintf("Current velocity %fm/s exceeds maximum train velocity %fm/s", new.Velocity, train.MaxVelocity))
		err := warnings.Append(OutOfBounds{Type: VelocityError, Max: c.train.MaxVelocity, Value: new.Velocity})
		if err != nil {
			log.Warning(fmt.Sprintf("%+v", err))
			return Sensors{}, err
		}
	}
	if new.Velocity > track.MaxVelocity {
		log.Warning(fmt.Sprintf("Current velocity %fm/s exceeds maximum track velocity %fm/s", new.Velocity, track.MaxVelocity))
		err = warnings.Append(OutOfBounds{Type: VelocityError, Max: track.MaxVelocity, Value: new.Velocity})
		if err != nil {
			log.Warning(fmt.Sprintf("%+v", err))
			return Sensors{}, err
		}
	}

	// Position
	new.Position = prev.Position + 0.5*(prev.Velocity+new.Velocity)*deltaSec

	// Relative position
	if beginNewTrack {
		new.RelPosition = 0.5 * (prev.Velocity + new.Velocity) * deltaSec
	} else {
		new.RelPosition = prev.RelPosition + 0.5*(prev.Velocity+new.Velocity)*deltaSec
	}

	// Slope
	new.Slope = track.Slope

	// Bend Radius
	new.BendRadius = track.BendRadius

	// Tunnel
	new.Tunnel = track.Tunnel

	// Slope resistance
	new.SlopeRes = new.Mass * gravity * math.Sin(new.Slope)

	// Basic resistance does not exist if train is a zero slope track
	// TODO: Add some tolerance.
	if new.Slope != 0.0 || new.Velocity != 0.0 {
		new.BasicRes = new.Mass * (train.ResistanceLin + train.ResistanceQua*new.Velocity*new.Velocity)
	}

	// Curve resistance only applies if train is moving
	if new.Velocity != 0.0 {
		if track.BendRadius <= 100 {
			// TODO: Why this value 100?
			// Prompt an Alert here? Danger!
		} else if track.BendRadius < 300 {
			new.CurveRes = 4.91 * new.Mass / (new.BendRadius - 55)
		} else {
			new.CurveRes = 6.3 * new.Mass / (new.BendRadius - 55)
		}
	}

	// Tunnel resistance
	if track.Tunnel {
		new.TunnelRes = 1.296 * 1e-9 * math.Max(track.Length-new.RelPosition, 0.0) * gravity * new.Velocity * new.Velocity
	}

	// Line resistance
	new.LineRes = new.SlopeRes + new.CurveRes + new.TunnelRes
	new.Resistance = new.BasicRes + new.LineRes

	// Acceleration
	maxAcceleration := (train.MaxTraction - new.Resistance) / (new.Mass * train.MassFactor)
	maxDeceleration := ((-1)*train.MaxBrake - new.Resistance) / (new.Mass * train.MassFactor)
	setpoint := sp.Value
	if setpoint > 0.0 && setpoint > maxAcceleration {
		log.Warning(fmt.Sprintf("Acceleration setpoint %fm/s2 exceeds maximum acceleration %fm/s2. Correction required", setpoint, maxAcceleration))
		err := warnings.Append(OutOfBounds{Type: AccelerationError, Min: maxDeceleration, Max: maxAcceleration, Value: setpoint})
		if err != nil {
			log.Warning(fmt.Sprintf("%+v", err))
			return Sensors{}, err
		}
		new.Acceleration = maxAcceleration

	} else if setpoint < 0.0 && setpoint < maxDeceleration {
		// Case setpoint being emergency brake not considered as a warning
		if setpoint != math.Inf(-1) {
			log.Warning(fmt.Sprintf("Deceleration setpoint %fm/s2 exceeds maximum deceleration %fm/s2. Correction required", setpoint, maxDeceleration))
			err := warnings.Append(OutOfBounds{Type: AccelerationError, Min: maxDeceleration, Max: maxAcceleration, Value: setpoint})
			if err != nil {
				log.Warning(fmt.Sprintf("%+v", err))
				return Sensors{}, err
			}
		}
		new.Acceleration = maxDeceleration
	} else {
		// Setpoint within limits
		new.Acceleration = setpoint
	}
	if setpoint < 0.0 && new.Velocity < 0.01 { // TODO: Remove  0.01 hardcode
		// Reverse gear not allowed.
		new.Acceleration = 0
		new.Velocity = 0
	}

	// Reverse
	if new.Velocity == 0.01 && new.Acceleration < 0.0 {
		new.Acceleration = 0.0
	}

	// Force & power
	force := new.Mass*train.MassFactor*new.Acceleration + new.Resistance
	if force >= 0 {
		if force > train.MaxTraction {
			log.Warning(fmt.Sprintf("Traction force %fN exceeds maximum traction force %fN. Correction required", force, train.MaxTraction))
			err := warnings.Append(OutOfBounds{Type: ForceError, Max: train.MaxTraction, Value: force})
			if err != nil {
				log.Warning(fmt.Sprintf("%+v", err))
				return Sensors{}, err
			}
			force = train.MaxTraction
		}
		new.TractionForce = force
		new.TractionPower = new.TractionForce * new.Velocity
		new.BrakingForce = 0
		new.BrakingPower = 0

	} else {
		if -force > train.MaxBrake {
			log.Warning(fmt.Sprintf("Braking force %fN exceeds maximum braking force %fN. Correction required", -force, train.MaxBrake))
			err := warnings.Append(OutOfBounds{Type: ForceError, Max: train.MaxBrake, Value: -force})
			if err != nil {
				log.Warning(fmt.Sprintf("%+v", err))
				return Sensors{}, err
			}
			force = train.MaxBrake
		}
		new.TractionForce = 0
		new.TractionPower = 0
		new.BrakingForce = -force
		new.BrakingPower = new.BrakingForce * new.Velocity
	}

	// Check Heartbeat
	// TODO: Remove hardcoded duration
	updateTimeout := time.Duration(5) * time.Second
	setpointElapsed := until.Sub(prev.Setpoint.Time)
	if setpointElapsed >= updateTimeout {
		alarms.Append(Heartbeat{
			LastTime:  sp.Time,
			Threshold: updateTimeout})
	}

	if warnings.Any() {
		new.Warnings = warnings
	}

	if alarms.Any() {
		new.Alarms = alarms
	}

	// Update
	c.sensors = new

	return new, nil
}

// EmergencyBrakeSetpoint returns the setpoint that activates emergency brakes.
func (c *Core) EmergencyBrakeSetpoint() Setpoint {
	return Setpoint{
		Value: math.Inf(-1),
		Time:  time.Now(),
	}
}
