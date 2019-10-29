package core_test

import (
	"encoding/json"
	"flag"
	"fmt"
	"log/syslog"
	"os"
	"testing"

	"github.com/google/go-cmp/cmp"
	log "github.com/trencat/goutils/syslog"
	"github.com/trencat/train/testutils"
)

// Test configuration
var flagUpdate bool

func TestMain(m *testing.M) {
	// Parse arguments
	flag.BoolVar(&flagUpdate, "update", false, "Update golden file tests.")
	flag.Parse()

	// Setup
	err := log.SetLogger("tcp", "localhost", "514",
		syslog.LOG_WARNING|syslog.LOG_LOCAL0, "coreTest")

	if err != nil {
		panic(fmt.Sprintf("%s", err))
	}

	//Teardown
	os.Exit(m.Run())
}

// TestUpdateSensorsAcceleration tests UpdateSensors implementation
// considering that setpoint refers to acceleration.
func TestUpdateSensorsAcceleration(t *testing.T) {
	testdata := testutils.GetUpdateSensorsAs(t)

	for alias, test := range testdata {
		//Read scenario
		scenario := testutils.GetScenario(test.Scenario, t)
		co := testutils.NewCore(scenario, t)

		newSensor, err := co.UpdateSensorsAcceleration(test.Setpoint, test.Expected.Time)
		if err != nil {
			t.Errorf("With scenario %s, Got error %s, Expected nil", alias, err)
			continue
		}

		if flagUpdate {
			// Update scenario expected value
			test.Expected = newSensor

			// Update testdata value
			testdataScenario := testdata[alias]
			testdataScenario.Expected = newSensor
			testdata[alias] = testdataScenario
		}

		if !cmp.Equal(test.Expected, newSensor) {
			t.Errorf("With scenario %s, Got Sensors%+v, Expected Sensors%+v", alias, test.Expected, newSensor)
			continue
		}
	}

	if flagUpdate {
		testutils.MarshalToFile(testutils.UpdateSensorsAPath, testdata, t)
	}
}

// TestUpdateScenarios used only for internal purposes.
func TestUpdateScenarios(t *testing.T) {
	testdata := testutils.GetScenarios(t)

	for alias, scenario := range testdata {
		//Read scenario
		sensors := testutils.ComputeSensors(scenario, t)

		data, err := json.Marshal(sensors)
		if err != nil {
			t.Fatalf("%+v", err)
		}

		fmt.Printf("%s\t%s\n ", alias, data)
	}
}
