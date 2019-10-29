// Package testutils provides shortcut methods to be used in testing.
package testutils

import (
	"encoding/json"
	"io/ioutil"
	"os"
	"path/filepath"
	"testing"

	"github.com/trencat/train/atp"
	"github.com/trencat/train/core"
)

// ScenariosPath is the path of testdata/scenarios.json
var ScenariosPath string

// TrainsPath is the path of testdata/trains.json
var TrainsPath string

// RoutesPath is the path of testdata/routes.json
var RoutesPath string

// UpdateSensorsAPath is the path of
// testdata/updateSensorsAcceleration.json
var UpdateSensorsAPath string

// Trains represents data in testdata/trains.json
type Trains map[string]core.Train

// Routes represents data in testdata/routes.json
type Routes map[string][]core.Track

type Scenario struct {
	Train   string
	Route   string
	Sensors core.Sensors
}

// Scenarios represents data in testdata/scenarios.json
type Scenarios map[string]Scenario

type UpdateSensorsA struct {
	Scenario string
	Setpoint core.Setpoint
	Expected core.Sensors
}

// UpdateSensorsAs represents data in
// testdata/updateSensorsAcceleration.json
type UpdateSensorsAs map[string]UpdateSensorsA

// init() is called when importing the package
func init() {
	// Read environment variables
	gopath := os.Getenv("GOPATH")
	if gopath == "" {
		panic("GOPATH environment variable is not set")
	}

	// Setup testdata paths
	testdataDir := filepath.Join(gopath, "src", "github.com", "trencat",
		"train", "testutils", "testdata")
	ScenariosPath = filepath.Join(testdataDir, "scenarios.json")
	TrainsPath = filepath.Join(testdataDir, "trains.json")
	RoutesPath = filepath.Join(testdataDir, "routes.json")
	UpdateSensorsAPath = filepath.Join(
		testdataDir, "updateSensorsAcceleration.json")
}

// NewAtp returns an atp.Atp instance with train, route and
// initial conditions set.
func NewAtp(scenario Scenario, t *testing.T) *atp.Atp {
	t.Helper()

	train := GetTrain(scenario.Train, t)
	route := GetRoute(scenario.Route, t)

	Atp, err := atp.New(train, route, scenario.Sensors)
	if err != nil {
		t.Fatalf("Cannot build atp. %+v", err)
	}

	return Atp
}

// NewCore returns a core.Core instance with train, route and
// initial conditions set.
func NewCore(scenario Scenario, t *testing.T) *core.Core {
	t.Helper()

	train := GetTrain(scenario.Train, t)
	route := GetRoute(scenario.Route, t)

	co, err := core.New(train, route, scenario.Sensors)
	if err != nil {
		t.Fatalf("Cannot build core. %+v", err)
	}

	return &co
}

// MarshalToFile encodes a mapping into a json file.
func MarshalToFile(path string, v interface{}, t *testing.T) {
	t.Helper()

	// marshal
	data, err := json.Marshal(v)
	if err != nil {
		t.Fatalf("%+v", err)
	}

	// write file
	err = ioutil.WriteFile(path, data, 0644)
	if err != nil {
		t.Fatalf("%+v", err)
	}
}

// UnmarshalFromFile decodes a json file into v variable.
func UnmarshalFromFile(path string, v interface{}, t *testing.T) {
	t.Helper()

	// read file
	data, err := ioutil.ReadFile(path)
	if err != nil {
		t.Fatalf("%+v", err)
	}

	// unmarshal
	if err := json.Unmarshal(data, v); err != nil {
		t.Fatalf("%+v", err)
	}
}

// GetTrain returns a train by its alias from testdata/trains.json
func GetTrain(alias string, t *testing.T) core.Train {
	t.Helper()

	testdata := GetTrains(t)
	train, exists := testdata[alias]
	if !exists {
		t.Fatalf("Train %s does not exist", alias)
	}

	return train
}

// GetTrains returns all trains from testdata/trains.json
func GetTrains(t *testing.T) Trains {
	t.Helper()

	testdata := make(Trains)
	UnmarshalFromFile(TrainsPath, &testdata, t)
	return testdata
}

// GetRoute returns a route by its alias from testdata/routes.json
func GetRoute(alias string, t *testing.T) []core.Track {
	t.Helper()

	testdata := GetRoutes(t)
	route, exists := testdata[alias]
	if !exists {
		t.Fatalf("Route %s does not exist", alias)
	}

	return route
}

// GetRoutes returns all routes from testdata/routes.json
func GetRoutes(t *testing.T) Routes {
	t.Helper()

	testdata := make(Routes)
	UnmarshalFromFile(RoutesPath, &testdata, t)
	return testdata
}

// GetScenario returns a Scenario by its alias from testdata/scenarios.json
func GetScenario(alias string, t *testing.T) Scenario {
	t.Helper()

	testdata := GetScenarios(t)
	scenario, exists := testdata[alias]
	if !exists {
		t.Fatalf("Scenario %s does not exist", alias)
	}

	return scenario
}

// GetScenarios returns all scenarios from testdata/scenarios.json
func GetScenarios(t *testing.T) Scenarios {
	t.Helper()

	testdata := make(Scenarios)
	UnmarshalFromFile(ScenariosPath, &testdata, t)
	return testdata
}

// GetUpdateSensorsAs returns all UpdateSensorsA cases from
// testdata/updateSensorsAcceleration.json
func GetUpdateSensorsAs(t *testing.T) UpdateSensorsAs {
	t.Helper()

	testdata := make(UpdateSensorsAs)
	UnmarshalFromFile(UpdateSensorsAPath, &testdata, t)
	return testdata
}

// ComputeSensors computes all sensors values given only
// Position, Velocity, Acceleration, Time, RelPosition
// and Setpoint. Values for NumPassengers are optional.
// This method is useful to compute testdata.
func ComputeSensors(scenario Scenario, t *testing.T) core.Sensors {
	t.Helper()

	co := NewCore(scenario, t)
	sensors, err := co.UpdateSensors(scenario.Sensors.Setpoint, scenario.Sensors.Time)
	if err != nil {
		t.Fatalf("%+v", err)
	}

	return sensors
}
