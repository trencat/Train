// Package testutils provides shortcut methods to be used in testing.
package testutils

import (
	"encoding/json"
	"io/ioutil"
	"log/syslog"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/trencat/train/atp"

	"github.com/trencat/train/core"
)

// ScenariosPath is the path of testdata/scenarios.json
var ScenariosPath string

// TrainsPath is the path of testdata/trains.json
var TrainsPath string

// TracksPath is the path of testdata/tracks.json
var TracksPath string

// UpdateSensorsAPath is the path of
// testdata/updateSensorsAcceleration.json
var UpdateSensorsAPath string

// Trains represents data in testdata/trains.json
type Trains map[string]core.Train

// Tracks represents data in testdata/tracks.json
type Tracks map[string][]core.Track

type Scenario struct {
	Train   string
	Track   string
	Sensors core.Sensors
}

// Scenarios represents data in testdata/scenarios.json
type Scenarios map[string]Scenario

type UpdateSensorsA struct {
	Scenario string
	Setpoint core.Setpoint
	Duration time.Duration
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
	TracksPath = filepath.Join(testdataDir, "tracks.json")
	UpdateSensorsAPath = filepath.Join(
		testdataDir, "updateSensorsAcceleration.json")
}

// NewAtp returns an atp.Atp instance with train, tracks and
// initial conditions set.
func NewAtp(scenario Scenario, log *syslog.Writer, t *testing.T) *atp.Atp {
	t.Helper()

	train := GetTrain(scenario.Train, t)
	track := GetTrack(scenario.Track, t)

	Atp, err := atp.New(train, track, scenario.Sensors, log)
	if err != nil {
		t.Fatalf("Cannot build atp. %+v", err)
	}

	return Atp
}

// NewCore returns a core.Core instance with train, tracks and
// initial conditions set.
func NewCore(scenario Scenario, log *syslog.Writer, t *testing.T) *core.Core {
	t.Helper()

	train := GetTrain(scenario.Train, t)
	track := GetTrack(scenario.Track, t)

	co, err := core.New(train, track, scenario.Sensors, log)
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

// GetTrack returns a track list by its alias from testdata/tracks.json
func GetTrack(alias string, t *testing.T) []core.Track {
	t.Helper()

	testdata := GetTracks(t)
	tracks, exists := testdata[alias]
	if !exists {
		t.Fatalf("Track %s does not exist", alias)
	}

	return tracks
}

// GetTracks returns all tracks from testdata/tracks.json
func GetTracks(t *testing.T) Tracks {
	t.Helper()

	testdata := make(Tracks)
	UnmarshalFromFile(TracksPath, &testdata, t)
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
// Trackindex and Setpoint. Values for NumPassengers are optional.
// This method is useful to compute testdata.
func ComputeSensors(scenario Scenario, log *syslog.Writer, t *testing.T) core.Sensors {
	t.Helper()

	co := NewCore(scenario, log, t)
	sensors, err := co.UpdateSensors(co, scenario.Sensors.Setpoint, 0, 0)
	if err != nil {
		t.Fatalf("%+v", err)
	}

	return sensors
}
