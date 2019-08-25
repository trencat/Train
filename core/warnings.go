package core

import (
	"fmt"
	"time"

	"github.com/pkg/errors"
)

const (
	VelocityError     string = "Velocity"
	AccelerationError string = "Acceleration"
	ForceError        string = "Force"
)

// Heartbeat contains information about the error recorded by Sensors.
type Heartbeat struct {
	LastTime  time.Time
	Threshold time.Duration
}

func (hb *Heartbeat) Error() string {
	return fmt.Sprintf(
		"Setpoint not received for more than %d. Last time was %s.",
		hb.Threshold, hb.LastTime)
}

// OutOfBounds contains information about the error recorded by Sensors.
type OutOfBounds struct {
	Type  string
	Min   float64
	Max   float64
	Value float64
}

// Error implements error interface
func (out OutOfBounds) Error() string {
	return fmt.Sprintf("%s %f out of bounds. Min: %f, Max: %f.",
		out.Type, out.Value, out.Min, out.Max)
}

// Warnings contains errors recorded by Sensors.
type Warnings struct {
	OutOfBounds []OutOfBounds
	Heartbeat   []Heartbeat
}

// Any returns true if it contains at least one error of any kind.
func (w *Warnings) Any() bool {
	return len(w.OutOfBounds) >= 1 || len(w.Heartbeat) >= 1
}

// Append adds an error to the Warnings struct.
func (w *Warnings) Append(v interface{}) error {
	switch value := v.(type) {
	case OutOfBounds:
		w.OutOfBounds = append(w.OutOfBounds, value)
		return nil
	case Heartbeat:
		w.Heartbeat = append(w.Heartbeat, value)
		return nil
	default:
		err := errors.Errorf("Cannot append %+v to Warnings", v)
		return err
	}
}
