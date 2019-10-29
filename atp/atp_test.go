package atp_test

import (
	"fmt"
	"log/syslog"
	"os"
	"testing"
	"time"

	log "github.com/trencat/goutils/syslog"
	"github.com/trencat/train/atp"
	"github.com/trencat/train/core"
	"github.com/trencat/train/testutils"
)

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
	err := log.SetLogger("tcp", "localhost", "514",
		syslog.LOG_WARNING|syslog.LOG_LOCAL0, "atpTest")

	if err != nil {
		panic(fmt.Sprintf("%s", err))
	}

	//Teardown
	os.Exit(m.Run())
}

// TestOn tests train finishes execution after calling Stop() when
// state is On.
func TestOn(t *testing.T) {
	alias := "stationary_flat"
	scenario := testutils.GetScenario(alias, t)
	Atp := testutils.NewAtp(scenario, t)

	Atp.Stop()
	time.Sleep(refreshRate)

	if state := Atp.Sensors().State; state != atp.Off {
		t.Errorf("With scenario %s, Got state %d, Expected %d",
			alias, state, atp.Off)
	}
}

// TestStartError tests atp.Start() method returns error when called
// before method atp.Set.
func TestStartError(t *testing.T) {
	alias := "stationary_flat"
	scenario := testutils.GetScenario(alias, t)
	Atp := testutils.NewAtp(scenario, t)
	defer Atp.Kill()

	if err := Atp.Start(); err == nil {
		t.Errorf("With scenario %s, Got nil error, Expected non nil error", alias)
	}
}

// TestActive tests state is set to Active after calling Start()
func TestActive(t *testing.T) {
	alias := "stationary_flat"
	scenario := testutils.GetScenario(alias, t)
	Atp := testutils.NewAtp(scenario, t)
	defer Atp.Kill()

	Atp.SetSetpoint(scenario.Sensors.Setpoint)
	Atp.Start()
	time.Sleep(refreshRate)

	if state := Atp.Sensors().State; state != atp.Active {
		t.Errorf("With scenario %s, Got state %d, Expected %d",
			alias, state, atp.Active)
	}
}

// TestActiveMovement tests that train moves when state is Active.
func TestActiveMovement(t *testing.T) {
	alias := "stationary_flat"
	scenario := testutils.GetScenario(alias, t)
	Atp := testutils.NewAtp(scenario, t)
	defer Atp.Kill()

	before := Atp.Sensors().Sensors

	Atp.SetSetpoint(core.Setpoint{Value: 0.5, Time: time.Now()})
	Atp.Start()

	time.Sleep(refreshRate)
	after := Atp.Sensors().Sensors

	if after.Position <= before.Position {
		t.Errorf("With scenario %s, Got before %+v, After %+v, "+
			"Expected after position greater than before position",
			alias, before, after)
	}
}

// TestActiveMovementStop tests state is set to Alarm when calling
// atp.Stop() while the train is moving
func TestActiveMovementStop(t *testing.T) {
	alias := "moving_flat"
	scenario := testutils.GetScenario(alias, t)
	Atp := testutils.NewAtp(scenario, t)
	defer Atp.Kill()

	Atp.SetSetpoint(scenario.Sensors.Setpoint)
	Atp.Start()

	Atp.Stop()
	time.Sleep(refreshRate)
	if state := Atp.Sensors().State; state != atp.Alarm {
		t.Errorf("With scenario %s, Got state %d. Expected state %d.",
			alias, state, atp.Alarm)
	}
}

// TestActiveStop tests train finishes execution when calling Stop
// while train has state Active and is stopped.
func TestActiveStop(t *testing.T) {
	alias := "stationary_flat"
	scenario := testutils.GetScenario(alias, t)
	Atp := testutils.NewAtp(scenario, t)
	defer Atp.Kill()

	Atp.SetSetpoint(scenario.Sensors.Setpoint)
	Atp.Start()
	time.Sleep(refreshRate)
	Atp.Stop()
	time.Sleep(refreshRate)

	if state := Atp.Sensors().State; state != atp.Off {
		t.Errorf("With scenario %s, Got state %d, Expected %d",
			alias, state, atp.Off)
	}
}

// TestActiveVelocityOverrun tests state is set to Warning when
// running more than permitted. Next it tests state is set from
// Warning to Active after reducing speed.
func TestActiveVelocityOverrun(t *testing.T) {
	alias := "velocity_limit"
	scenario := testutils.GetScenario(alias, t)
	Atp := testutils.NewAtp(scenario, t)
	defer Atp.Kill()

	Atp.SetSetpoint(core.Setpoint{Value: 0.15, Time: time.Now()})
	Atp.Start()
	time.Sleep(refreshRate)

	if state := Atp.Sensors().State; state != atp.Warning {
		t.Errorf("With scenario %s, Got state %d, Expected %d",
			alias, state, atp.Warning)
	}

	Atp.SetSetpoint(core.Setpoint{Value: -0.7, Time: time.Now()})
	time.Sleep(2 * refreshRate)

	if state := Atp.Sensors().State; state != atp.Active {
		t.Errorf("With scenario %s, Got state %d, Expected %d",
			alias, state, atp.Active)
	}
}

// TestActiveAccelerationOOB tests state is set to Warning when
// acceleration setpoint is out of bounds.
func TestActiveAccelerationOOB(t *testing.T) {
	alias := "stationary_ascend"
	scenario := testutils.GetScenario(alias, t)
	Atp := testutils.NewAtp(scenario, t)
	defer Atp.Kill()

	Atp.SetSetpoint(core.Setpoint{Value: 10, Time: time.Now()})
	Atp.Start()
	time.Sleep(refreshRate)

	if state := Atp.Sensors().State; state != atp.Warning {
		t.Errorf("With scenario %s, Got state %d, Expected %d",
			alias, state, atp.Warning)
	}
}

// TestSetpointTimeout tests state is set to Alarm if no setpoint
// is sent after X seconds.
func TestSetpointTimeout(t *testing.T) {
	alias := "velocity_limit"
	scenario := testutils.GetScenario(alias, t)
	Atp := testutils.NewAtp(scenario, t)
	defer Atp.Kill()

	Atp.SetSetpoint(core.Setpoint{Value: 0, Time: time.Now()})
	Atp.Start()
	time.Sleep(setpointTimeout)

	if state := Atp.Sensors().State; state != atp.Alarm {
		t.Errorf("With scenario %s, Got state %d, Expected %d",
			alias, state, atp.Alarm)
	}
}

// TestWarningAlarm tests state is set to Alarm if Warning state
// holds for more than X seconds
func TestWarningAlarm(t *testing.T) {
	alias := "velocity_limit"
	scenario := testutils.GetScenario(alias, t)
	Atp := testutils.NewAtp(scenario, t)
	defer Atp.Kill()

	Atp.SetSetpoint(core.Setpoint{Value: 0.1, Time: time.Now()})
	Atp.Start()
	time.Sleep(warningTimeout)

	if state := Atp.Sensors().State; state != atp.Alarm {
		t.Errorf("With scenario %s, Got state %d, Expected %d",
			alias, state, atp.Alarm)
	}
}

// TestAlarmSetpoints tests that setpoint is ignored if state is
// set to Alarm, train stops completely and state changes to On.
func TestAlarm(t *testing.T) {
	alias := "velocity_limit_alarm"
	scenario := testutils.GetScenario(alias, t)
	Atp := testutils.NewAtp(scenario, t)
	defer Atp.Kill()

	Atp.SetSetpoint(scenario.Sensors.Setpoint)
	Atp.Start()
	time.Sleep(refreshRate)

	// Check state is set to Alarm
	if state := Atp.Sensors().State; state != atp.Alarm {
		t.Fatalf("With scenario %s, Got state %d, Expected %d",
			alias, state, atp.Alarm)
	}

	// Wait until train stops
	prev := Atp.Sensors()
	for {
		// Atp should ignore setpoint
		Atp.SetSetpoint(scenario.Sensors.Setpoint)

		time.Sleep(refreshRate)
		now := Atp.Sensors()

		// Check that train is braking
		if now.Sensors.Velocity >= prev.Sensors.Velocity {
			t.Fatalf("With scenario %s, Got previous %+v, current %+v, expected train to brake",
				alias, prev, now)
		}

		if atp.Stopped(now.Sensors) {
			break
		}
	}

	time.Sleep(refreshRate)
	if state := Atp.Sensors().State; state != atp.On {
		t.Errorf("With scenario %s, Got state %d, Expected %d",
			alias, state, atp.On)
	}
}

// TestPanicOutOfRails tests that train panics when running out of
// rails.
func TestPanicOutOfRails(t *testing.T) {
	// TODO
}
