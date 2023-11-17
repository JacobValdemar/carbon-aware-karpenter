package common

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"
)

const BoavizapiBaseURL string = "http://localhost:5001" // or https://dev.api.boavizta.org

type VerboseInfo struct {
	NodeName     string  `json:"NodeName"`
	InstanceType string  `json:"InstanceType"`
	Utilization  float64 `json:"Utilization"`
}

type Data struct {
	Verbose []VerboseInfo `json:"Verbose"`
}

// go build analyzer.go && sudo cp analyzer /usr/local/bin/
func analyzer(data []NodeUtil) float64 {

	var totalCarbonEmissions float64

	for _, node := range data {
		location := nodeNameToLocation(node.NodeName, "IRL")
		embodied, operational, _, _ := getEmissions(node.InstanceType, location, node.Utilization*100)
		totalCarbonEmissions += embodied
		totalCarbonEmissions += operational
	}

	//fmt.Printf("%f g CO2e per hour\n", totalCarbonEmissions*1000)
	return totalCarbonEmissions * 1000
}

func nodeNameToLocation(nodeName string, defaulLocation string) string {
	var locationToRegion map[string][]string = map[string][]string{
		"USA": {"us-east-1", "us-east-2", "us-west-1", "us-west-2"},
		"ZAF": {"af-south-1"},
		"HKG": {"ap-east-1"},
		"IND": {"ap-south-1", "ap-south-2"},
		"JPN": {"ap-northeast-1", "ap-northeast-3"},
		"KOR": {"ap-northeast-2"},
		"SGP": {"ap-southeast-1"},
		"AUS": {"ap-southeast-2", "ap-southeast-4"},
		"IDN": {"ap-southeast-3"},
		"CAN": {"ca-central-1"},
		"DEU": {"eu-central-1"},
		"CHE": {"eu-central-2"},
		"IRL": {"eu-west-1"},
		"GBR": {"eu-west-2"},
		"FRA": {"eu-west-3"},
		"ITA": {"eu-south-1"},
		"ESP": {"eu-south-2"},
		"SWE": {"eu-north-1"},
		"ISR": {"il-central-1"},
		"BHR": {"me-south-1"},
		"ARE": {"me-central-1"},
		"BRA": {"sa-east-1"},
	}

	for a, b := range locationToRegion {
		for _, z := range b {
			if strings.Contains(nodeName, z) {
				return a
			}
		}
	}

	fmt.Printf("Region '%s' not recognized. Using default location: '%s'.\n", nodeName, defaulLocation)
	return defaulLocation
}

func getEmissions(instanceType string, usageLocation string, load_percentage float64) (float64, float64, float64, float64) {
	url := BoavizapiBaseURL + "/v1/cloud/instance?verbose=true&duration=1&criteria=gwp"

	payload := []byte(`{
		"provider": "aws",
		"instance_type": "` + instanceType + `",
		"usage": {
		  "usage_location": "` + usageLocation + `",
		  "time_workload": [
			{
			  "time_percentage": 100,
			  "load_percentage": ` + strconv.FormatFloat(load_percentage, 'f', -1, 64) + `
			}
		  ]
		}
	  }`)

	resp, err := http.Post(url, "application/json", bytes.NewBuffer(payload))
	if err != nil {
		panic(err)
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		panic(err)
	}

	var response ResponseGenerated
	err = json.Unmarshal([]byte(body), &response)
	if err != nil {
		panic(err)
	}

	return response.Impacts.Gwp.Embedded.Value,
		response.Impacts.Gwp.Use.Value,
		(response.Verbose.CPU1.CoreUnits.Value * response.Verbose.CPU1.Units.Value) / response.Verbose.InstancePerServer.Value,
		(response.Verbose.RAM1.Capacity.Value * response.Verbose.RAM1.Units.Value) / response.Verbose.InstancePerServer.Value
}

type ResponseGenerated struct {
	Impacts struct {
		Gwp struct {
			Embedded struct {
				Value              float64  `json:"value,omitempty"`
				SignificantFigures int      `json:"significant_figures,omitempty"`
				Min                float64  `json:"min,omitempty"`
				Max                float64  `json:"max,omitempty"`
				Warnings           []string `json:"warnings,omitempty"`
			} `json:"embedded,omitempty"`
			Use struct {
				Value              float64 `json:"value,omitempty"`
				SignificantFigures int     `json:"significant_figures,omitempty"`
				Min                float64 `json:"min,omitempty"`
				Max                float64 `json:"max,omitempty"`
			} `json:"use,omitempty"`
			Unit        string `json:"unit,omitempty"`
			Description string `json:"description,omitempty"`
		} `json:"gwp,omitempty"`
	} `json:"impacts,omitempty"`
	Verbose struct {
		CPU1 struct {
			CoreUnits struct {
				Value float64 `json:"value,omitempty"`
			} `json:"core_units,omitempty"`
			Units struct {
				Value float64 `json:"value,omitempty"`
			} `json:"units,omitempty"`
		} `json:"CPU-1,omitempty"`
		RAM1 struct {
			Capacity struct {
				Value float64 `json:"value,omitempty"`
			} `json:"capacity,omitempty"`
			Units struct {
				Value float64 `json:"value,omitempty"`
			} `json:"units,omitempty"`
		} `json:"RAM-1,omitempty"`
		InstancePerServer struct {
			Value float64 `json:"value,omitempty"`
		} `json:"instance_per_server,omitempty"`
	} `json:"verbose,omitempty"`
}
