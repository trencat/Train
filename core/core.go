// Package core provides a simple (yet complete) implementation
// of train movement.
package core

import (
	"fmt"
	"log/syslog"
	"math"
	"time"

	"github.com/pkg/errors"
)

const gravity float64 = 9.80665

// EmergencyBrakeSetpoint returns the setpoint that activates emergency brakes.
type EmergencyBrakeSetpoint func() Setpoint

// Core collects essential information for train automation and
// implements train movement. Implements interfaces.Core.
type Core struct {
	train                  Train
	tracks                 []Track
	sensors                Sensors
	UpdateSensors          UpdateCoreSensorsA
	EmergencyBrakeSetpoint EmergencyBrakeSetpoint
	log                    *syslog.Writer
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
	TrackIndex    int // Current track position in core.tracks slice
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

type Setpoint struct {
	Value float64
	Time  time.Time
}

// UpdateCoreSensorsA updates core sensors after a given time duration.
// setpoint parameter refers to Acceleration
type UpdateCoreSensorsA func(c *Core, setpoint Setpoint,
	elapsed time.Duration, setpointElapsed time.Duration) (sensors Sensors, panic error)

// emergencyBrakeSetpoint returns the setpoint that activates emergency brakes.
func emergencyBrakeSetpoint() Setpoint {
	return Setpoint{
		Value: math.Inf(-1),
		Time:  time.Now(),
	}
}

// New initialises a Core instance.
func New(train Train, track []Track, sensors Sensors, log *syslog.Writer) (Core, error) {
	if log == nil {
		// Panic?
		err := errors.New("Attempt to declare a new Core. Log not provided (nil)")
		fmt.Printf("%+v", err)
		return Core{}, err
	}

	log.Info("New Core initialised")
	return Core{
		train:                  train,
		tracks:                 track,
		sensors:                sensors,
		UpdateSensors:          updateSensorsAcceleration,
		EmergencyBrakeSetpoint: emergencyBrakeSetpoint,
		log:                    log,
	}, nil
}

// Track returns Track specifications by its ID.
func (c *Core) Track(index int) (Track, error) {
	if c.tracks == nil {
		err := errors.New("Attempt to GetTrack. Core.tracks is (nil)")
		c.log.Warning(fmt.Sprintf("%+v", err))
		return Track{}, err
	}

	if index >= len(c.tracks) || index < 0 {
		err := errors.Errorf("Attempt to GetTrack. Position %d out of bounds", index)
		c.log.Warning(fmt.Sprintf("%+v", err))
		return Track{}, err
	}

	track := c.tracks[index]

	return track, nil
}

// UpdateSensors updates real time data after a given time duration.
// Setpoint argument refers to acceleration.
func updateSensorsAcceleration(c *Core, sp Setpoint,
	elapsed time.Duration, setpointElapsed time.Duration) (new Sensors, panic error) {
	//TODO: Remove hardcoded constants
	//TODO: Watch out, many log errors may happen.

	prev := &c.sensors
	train := &c.train

	track, err := c.Track(prev.TrackIndex)
	if err != nil {
		return Sensors{}, err
	}

	warnings := Warnings{}
	alarms := Warnings{}

	// TrackIndex
	new.TrackIndex = prev.TrackIndex

	// Update track
	beginNewTrack := (prev.RelPosition > track.Length)
	if beginNewTrack {
		new.TrackIndex = prev.TrackIndex + 1
		nextTrack, err := c.Track(new.TrackIndex)
		if err != nil {
			return Sensors{}, err
		}

		track = nextTrack
	}

	// TrackID
	new.TrackID = track.ID

	// Time
	deltaSec := elapsed.Seconds()
	new.Time = prev.Time.Add(elapsed)

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
		c.log.Warning(fmt.Sprintf("Current velocity %fm/s exceeds maximum train velocity %fm/s.", new.Velocity, train.MaxVelocity))
		err := warnings.Append(OutOfBounds{Type: VelocityError, Max: c.train.MaxVelocity, Value: new.Velocity})
		if err != nil {
			c.log.Warning(fmt.Sprintf("%+v", err))
			return Sensors{}, err
		}
	}
	if new.Velocity > track.MaxVelocity {
		c.log.Warning(fmt.Sprintf("Current velocity %fm/s exceeds maximum track velocity %fm/s.", new.Velocity, track.MaxVelocity))
		err = warnings.Append(OutOfBounds{Type: VelocityError, Max: track.MaxVelocity, Value: new.Velocity})
		if err != nil {
			c.log.Warning(fmt.Sprintf("%+v", err))
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
		c.log.Warning(fmt.Sprintf("Acceleration setpoint %fm/s2 exceeds maximum acceleration %fm/s2. Correction required.", setpoint, maxAcceleration))
		err := warnings.Append(OutOfBounds{Type: AccelerationError, Min: maxDeceleration, Max: maxAcceleration, Value: setpoint})
		if err != nil {
			c.log.Warning(fmt.Sprintf("%+v", err))
			return Sensors{}, err
		}
		new.Acceleration = maxAcceleration

	} else if setpoint < 0.0 && setpoint < maxDeceleration {
		// Case setpoint being emergency brake not considered as a warning
		if setpoint != math.Inf(-1) {
			c.log.Warning(fmt.Sprintf("Deceleration setpoint %fm/s2 exceeds maximum deceleration %fm/s2. Correction required.", setpoint, maxDeceleration))
			err := warnings.Append(OutOfBounds{Type: AccelerationError, Min: maxDeceleration, Max: maxAcceleration, Value: setpoint})
			if err != nil {
				c.log.Warning(fmt.Sprintf("%+v", err))
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
			c.log.Warning(fmt.Sprintf("Traction force %fN exceeds maximum traction force %fN. Correction required.", force, train.MaxTraction))
			err := warnings.Append(OutOfBounds{Type: ForceError, Max: train.MaxTraction, Value: force})
			if err != nil {
				c.log.Warning(fmt.Sprintf("%+v", err))
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
			c.log.Warning(fmt.Sprintf("Braking force %fN exceeds maximum braking force %fN. Correction required.", -force, train.MaxBrake))
			err := warnings.Append(OutOfBounds{Type: ForceError, Max: train.MaxBrake, Value: -force})
			if err != nil {
				c.log.Warning(fmt.Sprintf("%+v", err))
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
