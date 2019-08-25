package atp_test

import (
	"fmt"
	"log/syslog"
	"os"
	"testing"
	"time"

	"github.com/trencat/train/atp"
	"github.com/trencat/train/core"

	"github.com/trencat/train/testutils"
)

var log *syslog.Writer
var refreshRate time.Duration     // TODO: Remove from here and read from environ variable
var setpointTimeout time.Duration // TODO: Remove from here and read from environ variable
var warningTimeout time.Duration  // TODO: Remove from here and read from environ variable

func TestMain(m *testing.M) {
	// Parse args
	// TODO: Read refreshRate from environ vars.
	refreshRate = time.Duration(1) * time.Second
	setpointTimeout = time.Duration(7) * time.Second
	warningTimeout = time.Duration(7) * time.Second

	// Setup
	syslog, error := syslog.Dial("tcp", "localhost:514",
		syslog.LOG_WARNING|syslog.LOG_LOCAL0, "atpTest")

	if error != nil {
		panic(fmt.Sprintf("%s", error))
	}

	log = syslog

	//Teardown
	os.Exit(m.Run())
}

// TestOn tests train finishes execution after calling Stop() when
// status is On.
func TestOn(t *testing.T) {
	alias := "stationary_flat"
	scenario := testutils.GetScenario(alias, t)
	Atp := testutils.NewAtp(scenario, log, t)

	Atp.Stop()
	time.Sleep(refreshRate)

	if status := Atp.Status(); status != atp.Off {
		t.Errorf("With scenario %s, Got status %d, Expected %d",
			alias, status, atp.Off)
	}
}

// TestStartError tests atp.Start() method returns error when called
// before method atp.Set.
func TestStartError(t *testing.T) {
	alias := "stationary_flat"
	scenario := testutils.GetScenario(alias, t)
	Atp := testutils.NewAtp(scenario, log, t)
	defer Atp.Kill()

	if err := Atp.Start(); err == nil {
		t.Errorf("With scenario %s, Got nil error, Expected non nil error", alias)
	}
}

// TestActive tests status is set to Active after calling Start()
func TestActive(t *testing.T) {
	alias := "stationary_flat"
	scenario := testutils.GetScenario(alias, t)
	Atp := testutils.NewAtp(scenario, log, t)
	defer Atp.Kill()

	Atp.Set(scenario.Sensors.Setpoint)
	Atp.Start()
	time.Sleep(refreshRate)

	if status := Atp.Status(); status != atp.Active {
		t.Errorf("With scenario %s, Got status %d, Expected %d",
			alias, status, atp.Active)
	}
}

// TestActiveMovement tests that train moves when status is Active.
func TestActiveMovement(t *testing.T) {
	alias := "stationary_flat"
	scenario := testutils.GetScenario(alias, t)
	Atp := testutils.NewAtp(scenario, log, t)
	defer Atp.Kill()

	before := Atp.Sensors()

	Atp.Set(core.Setpoint{Value: 0.5, Time: time.Now()})
	Atp.Start()

	time.Sleep(refreshRate)
	after := Atp.Sensors()

	if after.Position <= before.Position {
		t.Errorf("With scenario %s, Got before %+v, After %+v, "+
			"Expected after position greater than before position",
			alias, before, after)
	}
}

// TestActiveMovementStop tests status is set to Alarm when calling
// atp.Stop() while the train is moving
func TestActiveMovementStop(t *testing.T) {
	alias := "moving_flat"
	scenario := testutils.GetScenario(alias, t)
	Atp := testutils.NewAtp(scenario, log, t)
	defer Atp.Kill()

	Atp.Set(scenario.Sensors.Setpoint)
	Atp.Start()

	Atp.Stop()
	time.Sleep(refreshRate)
	if status := Atp.Status(); status != atp.Alarm {
		t.Errorf("With scenario %s, Got status %d. Expected status %d.",
			alias, status, atp.Alarm)
	}
}

// TestActiveStop tests train finishes execution when calling Stop
// while train has status Active and is stopped.
func TestActiveStop(t *testing.T) {
	alias := "stationary_flat"
	scenario := testutils.GetScenario(alias, t)
	Atp := testutils.NewAtp(scenario, log, t)
	defer Atp.Kill()

	Atp.Set(scenario.Sensors.Setpoint)
	Atp.Start()
	time.Sleep(refreshRate)
	Atp.Stop()
	time.Sleep(refreshRate)

	if status := Atp.Status(); status != atp.Off {
		t.Errorf("With scenario %s, Got status %d, Expected %d",
			alias, status, atp.Off)
	}
}

// TestActiveVelocityOverrun tests status is set to Warning when
// running more than permitted. Next it tests status is set from
// Warning to Active after reducing speed.
func TestActiveVelocityOverrun(t *testing.T) {
	alias := "velocity_limit"
	scenario := testutils.GetScenario(alias, t)
	Atp := testutils.NewAtp(scenario, log, t)
	defer Atp.Kill()

	Atp.Set(core.Setpoint{Value: 0.15, Time: time.Now()})
	Atp.Start()
	time.Sleep(refreshRate)

	if status := Atp.Status(); status != atp.Warning {
		t.Errorf("With scenario %s, Got status %d, Expected %d",
			alias, status, atp.Warning)
	}

	Atp.Set(core.Setpoint{Value: -0.7, Time: time.Now()})
	time.Sleep(2 * refreshRate)

	if status := Atp.Status(); status != atp.Active {
		t.Errorf("With scenario %s, Got status %d, Expected %d",
			alias, status, atp.Active)
	}
}

// TestActiveAccelerationOOB tests status is set to Warning when
// acceleration setpoint is out of bounds.
func TestActiveAccelerationOOB(t *testing.T) {
	alias := "stationary_ascend"
	scenario := testutils.GetScenario(alias, t)
	Atp := testutils.NewAtp(scenario, log, t)
	defer Atp.Kill()

	Atp.Set(core.Setpoint{Value: 10, Time: time.Now()})
	Atp.Start()
	time.Sleep(refreshRate)

	if status := Atp.Status(); status != atp.Warning {
		t.Errorf("With scenario %s, Got status %d, Expected %d",
			alias, status, atp.Warning)
	}
}

// TestSetpointTimeout tests status is set to Alarm if no setpoint
// is sent after X seconds.
func TestSetpointTimeout(t *testing.T) {
	alias := "velocity_limit"
	scenario := testutils.GetScenario(alias, t)
	Atp := testutils.NewAtp(scenario, log, t)
	defer Atp.Kill()

	Atp.Set(core.Setpoint{Value: 0, Time: time.Now()})
	Atp.Start()
	time.Sleep(setpointTimeout)

	if status := Atp.Status(); status != atp.Alarm {
		t.Errorf("With scenario %s, Got status %d, Expected %d",
			alias, status, atp.Alarm)
	}
}

// TestWarningAlarm tests status is set to Alarm if Warning state
// holds for more than X seconds
func TestWarningAlarm(t *testing.T) {
	alias := "velocity_limit"
	scenario := testutils.GetScenario(alias, t)
	Atp := testutils.NewAtp(scenario, log, t)
	defer Atp.Kill()

	Atp.Set(core.Setpoint{Value: 0.1, Time: time.Now()})
	Atp.Start()
	time.Sleep(warningTimeout)

	if status := Atp.Status(); status != atp.Alarm {
		t.Errorf("With scenario %s, Got status %d, Expected %d",
			alias, status, atp.Alarm)
	}
}

// TestAlarmSetpoints tests that setpoint is ignored if state is
// set to Alarm, train stops completely and status changes to On.
func TestAlarm(t *testing.T) {
	alias := "velocity_limit_alarm"
	scenario := testutils.GetScenario(alias, t)
	Atp := testutils.NewAtp(scenario, log, t)
	defer Atp.Kill()

	Atp.Set(scenario.Sensors.Setpoint)
	Atp.Start()
	time.Sleep(refreshRate)

	// Check status is set to Alarm
	if status := Atp.Status(); status != atp.Alarm {
		t.Fatalf("With scenario %s, Got status %d, Expected %d",
			alias, status, atp.Alarm)
	}

	// Wait until train stops
	prev := Atp.Sensors()
	for {
		// Atp should ignore setpoint
		Atp.Set(scenario.Sensors.Setpoint)

		time.Sleep(refreshRate)
		sensors := Atp.Sensors()

		// Check that train is braking
		if sensors.Velocity >= prev.Velocity {
			t.Fatalf("With scenario %s, Got previus %+v, current %+v, expected train to brake",
				alias, prev, sensors)
		}

		if atp.Stopped(sensors) {
			break
		}
	}

	time.Sleep(refreshRate)
	if status := Atp.Status(); status != atp.On {
		t.Errorf("With scenario %s, Got status %d, Expected %d",
			alias, status, atp.On)
	}
}

// TestPanicOutOfRails tests that train panics when running out of
// rails.
func TestPanicOutOfRails(t *testing.T) {
	// TODO
}
