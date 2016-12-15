// Package collect throughput and response times using Go-Kit Metrics
package collect

import (
	"fmt"
	. "github.com/adrianco/goguesstimate/guesstimate"
	"github.com/adrianco/spigo/tooling/archaius"
	"github.com/adrianco/spigo/tooling/names"
	//"github.com/go-kit/kit/metrics"
	//"github.com/go-kit/kit/metrics/expvar"
	"github.com/go-kit/kit/metrics/generic"
	"log"
	"net"
	"net/http"
	"os"
	"sync"
	"time"
	"io/ioutil"
	"strings"
	"encoding/json"
)

const (
	maxHistObservable = 1000000 // one millisecond
	sampleCount       = 1000    // data points will be sampled 5000 times to build a distribution by guesstimate
)

type ArchObject struct {
	Arch string `json:"arch"`
	Version string `json:"version"`
	Args string `json:"args"`
	Services []struct {
		Name string `json:"name"`
		Package string `json:"package"`
		Regions int `json:"regions"`
		Count int `json:"count"`
		Dependencies []string `json:"dependencies"`
		UseCustomGuesstimate bool `json:"useCustomGuesstimate,omitempty"`
		GuesstimateType string `json:"guesstimateType,omitempty"`
		GuesstimateValue string `json:"guesstimateValue,omitempty"`
	} `json:"services"`
}
//save a sample of the actual data for use by guesstimate
var sampleMap map[*generic.Histogram][]int64
var sampleLock sync.Mutex

// NewHist creates a new histogram
func NewHist(name string) *generic.Histogram {
	var h *generic.Histogram
	if name != "" && archaius.Conf.Collect {
		h = generic.NewHistogram(name, 100) // 1000, maxHistObservable, 1, []int{50, 99}...)
		sampleLock.Lock()
		if sampleMap == nil {
			sampleMap = make(map[*generic.Histogram][]int64)
		}
		sampleMap[h] = make([]int64, 0, sampleCount)
		sampleLock.Unlock()
		return h
	}
	return nil
}

// Measure adds a measurement to a histogram collection
func Measure(h *generic.Histogram, d time.Duration) {
	if h != nil && archaius.Conf.Collect {
		if d > maxHistObservable {
			h.Observe(float64(maxHistObservable))
		} else {
			h.Observe(float64(d))
		}
		sampleLock.Lock()
		s := sampleMap[h]
		if s != nil && len(s) < sampleCount {
			sampleMap[h] = append(s, int64(d))
			sampleLock.Unlock()
		}
	}
}

// SaveHist passes in name because metrics.Histogram blocks expvar.Histogram.Name()
func SaveHist(h *generic.Histogram, name, suffix string) {
	if archaius.Conf.Collect {
		file, err := os.Create("csv_metrics/" + names.Arch(name) + "_" + names.Instance(name) + suffix + ".csv")
		if err != nil {
			log.Fatal("Save histogram %v: %v\n", name, err)
		}
		//metrics.PrintDistribution(file, h)
		h.Print(file)
		file.Close()
	}
}

// SaveAllGuesses writes guesses to a file
func SaveAllGuesses(name string) {
	if len(sampleMap) == 0 {
		return
	}
	log.Printf("Saving %v histograms for Guesstimate\n", len(sampleMap))
	var g Guess
	g = Guess{
		Space: GuessModel{
			Name:        names.Arch(name),
			Description: "Guesstimate generated by github.com/adrianco/spigo",
			IsPrivate:   "true",
			Graph: GuessGraph{
				Metrics:      make([]GuessMetric, 0, len(sampleMap)),
				Guesstimates: make([]Guesstimate, 0, len(sampleMap)),
			},
		},
	}

	var archObject ArchObject

	file, e := ioutil.ReadFile("./json_arch/" + names.Arch(name) + "_arch.json")
	if e != nil {
		log.Printf("File error: %v\n", e)
		os.Exit(1)
	}

	if file != nil {
		json.Unmarshal(file, &archObject)
	}

	row := 1
	col := 1
	seq := []string{"", "A", "B", "C", "D", "E", "F", "G", "H", "I", "J", "K", "L", "M", "N", "O", "P", "Q", "R", "S", "T", "U", "V", "W", "X", "Y", "Z"}
	for h, data := range sampleMap {

		UseCustomGuesstimate := false
		GuesstimateType := "DATA"
		GuesstimateValue := ""

		if(archObject.Arch != ""){
			for _, e := range archObject.Services {
				if(strings.Contains(h.Name, e.Name)){
					UseCustomGuesstimate = e.UseCustomGuesstimate
					GuesstimateType = e.GuesstimateType
					GuesstimateValue = e.GuesstimateValue
				}
			}
		}

		if UseCustomGuesstimate {
			data = nil
		}

		g.Space.Graph.Metrics = append(g.Space.Graph.Metrics, GuessMetric{
			ID:         seq[row] + seq[col],
			ReadableID: seq[row] + seq[col],
			Name:       h.Name,
			Location:   GuessMetricLocation{row, col},
		})
		
		g.Space.Graph.Guesstimates = append(g.Space.Graph.Guesstimates, Guesstimate{
			Metric:          seq[row] + seq[col],
			Expression:           GuesstimateValue,
			GuesstimateType: GuesstimateType,
			Data:            data,
		})
		row++
		if row >= len(seq) {
			row = 1
			col++
			if col >= len(seq) {
				break
			}
		}
	}
	SaveGuess(g, "json_metrics/"+names.Arch(name))
}

// Save currently does nothing
func Save() {
	//	if archaius.Conf.Collect {
	//		file, _ := os.Create("csv_metrics/" + archaius.Conf.Arch + "_metrics.csv")
	//		counters, gauges := metrics.Snapshot()
	//		cj, _ := json.Marshal(counters)
	//		gj, _ := json.Marshal(gauges)
	//		file.WriteString(fmt.Sprintf("{\n\"counters\":%v\n\"gauges\":%v\n}\n", string(cj), string(gj)))
	//		file.Close()
	//	}
}

// Serve on a port
func Serve(port int) {
	sock, err := net.Listen("tcp", fmt.Sprintf("localhost:%v", port))
	if err != nil {
		log.Fatal(err)
	}
	go func() {
		log.Printf("HTTP metrics now available at localhost:%v/debug/vars", port)
		http.Serve(sock, nil)
	}()
}
