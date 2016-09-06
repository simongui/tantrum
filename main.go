package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"os/exec"
	"strconv"
	"strings"
	"time"

	"github.com/dlion/goImgur"
	chart "github.com/wcharczuk/go-chart"
	"github.com/wcharczuk/go-chart/drawing"
	kingpin "gopkg.in/alecthomas/kingpin.v2"
)

type result struct {
	x float64
	y float64
}

var (
	verbose     = kingpin.Flag("verbose", "Verbose mode.").Short('v').Bool()
	hosts       = kingpin.Flag("hosts", "Host addresses for the target Redis servers to benchmark against.").Short('h').Required().String()
	image       = kingpin.Flag("image", "Where to store the results graph in PNG format.").Default("results.png").Short('i').String()
	requests    = kingpin.Flag("requests", "Number of total requests.").Short('r').Default("10000000").Uint32()
	connections = kingpin.Flag("connections", "Number of Redis client connections.").Short('c').Default("128").Uint16()
	pipelined   = kingpin.Flag("pipelined", "Number of pipelined requests per connection.").Short('p').Default("128").Uint16()

	colors = []drawing.Color{
		drawing.ColorBlue,
		drawing.ColorRed,
	}
)

func main() {
	kingpin.Parse()

	var series []chart.Series
	addresses := strings.Split(*hosts, ",")

	for index, address := range addresses {
		var offset = 0
		var name string
		hostParts := strings.Split(address, ":")
		if len(hostParts) > 2 {
			offset = 1
			name = hostParts[0]
		} else {
			name = hostParts[0] + " " + hostParts[1]
		}
		host := hostParts[0+offset]
		port := hostParts[1+offset]

		fmt.Printf("Running benchmark against %s on %s:%s\n", name, host, port)

		results, err := runBenchmark(host, port)
		if err != nil {
			fmt.Println(err)
		}

		s := parseResults(name, results)
		s.Style.StrokeColor = colors[index]
		series = append(series, s)

		if len(addresses) > 1 && index < len(addresses)-1 {
			fmt.Printf("Sleeping between runs for 5 seconds\n")
			time.Sleep(time.Second * 5)
		}
	}
	chartResults(series)
	url, err := postToImgur("results.png")
	if err != nil {
		fmt.Println(err)
	}
	fmt.Println(url)
}

func runBenchmark(host string, port string) (string, error) {
	cmd := exec.Command(
		"./redis-benchmark",
		"-h",
		host,
		"-p",
		port,
		"-t",
		"set",
		"-n",
		strconv.FormatUint(uint64(*requests), 10),
		"-c",
		strconv.FormatUint(uint64(*connections), 10),
		"-P",
		strconv.FormatUint(uint64(*pipelined), 10))

	output, err := cmd.CombinedOutput()
	return string(output), err
}

func parseResults(name string, results string) chart.ContinuousSeries {
	startResults := false
	endResults := false
	var xResults []float64
	var yResults []float64
	var lastYResult float64

	lines := strings.Split(results, "\n")
	for i := 0; i < len(lines)-4; i++ {
		line := lines[i]
		if line == "" && startResults == false {
			startResults = true
		} else if line == "" && startResults == true {
			endResults = true
		}

		if startResults && !endResults && len(line) > 0 {
			lineParts := strings.Split(line, "<=")
			percentileString := strings.Split(lineParts[0], "%")[0]
			latencyStringParts := strings.Split(lineParts[1], " ")

			percentile, _ := strconv.ParseFloat(percentileString, 64)
			latency, _ := strconv.ParseFloat(latencyStringParts[1], 64)

			xResults = append(xResults, percentile)
			yResults = append(yResults, latency)
			lastYResult = latency
		}
	}

	xResults = append(xResults, 101.0)
	yResults = append(yResults, lastYResult)

	nameString := fmt.Sprintf("%s max: %s at %s", name, lines[len(lines)-5], lines[len(lines)-4])
	fmt.Println(nameString)

	series := chart.ContinuousSeries{
		Style: chart.Style{
			Show:        true,              //note; if we set ANY other properties, we must set this to true.
			StrokeColor: drawing.ColorBlue, // will supercede defaults
			// FillColor:   drawing.ColorBlue.WithAlpha(64), // will supercede defaults
			StrokeDashArray: []float64{5.0, 5.0},
		},
		Name:    nameString,
		XValues: xResults,
		YValues: yResults,
	}
	return series
}

func chartResults(series []chart.Series) {
	graph := chart.Chart{
		XAxis: chart.XAxis{
			Name:      "Percentile",
			NameStyle: chart.StyleShow(),
			Style: chart.Style{
				Show:     true,
				FontSize: 12,
			},
		},
		YAxis: chart.YAxis{
			Name:      "Latency (milliseconds)",
			NameStyle: chart.StyleShow(),
			Style: chart.Style{
				Show:     true,
				FontSize: 12,
			},
		},
		Background: chart.Style{
			Padding: chart.Box{
				Top:    20,
				Left:   20,
				Right:  20,
				Bottom: 20,
			},
			FillColor: drawing.ColorFromHex("efefef"),
		},
		Canvas: chart.Style{
			FillColor:   drawing.ColorFromHex("efefef"),
			StrokeColor: drawing.ColorFromHex("efefef"),
		},
		Series: series,
		// Series: []chart.Series{
		// 	chart.ContinuousSeries{
		// 		Style: chart.Style{
		// 			Show:        true,              //note; if we set ANY other properties, we must set this to true.
		// 			StrokeColor: drawing.ColorBlue, // will supercede defaults
		// 			// FillColor:   drawing.ColorBlue.WithAlpha(64), // will supercede defaults
		// 			// StrokeDashArray: []float64{5.0, 5.0},
		// 		},
		// 		Name:    "redis",
		// 		XValues: []float64{50, 75, 90, 95, 99, 100},
		// 		YValues: []float64{1.0, 5.0, 10.0, 15.0, 50.0, 100.0},
		// 	},
		// 	chart.ContinuousSeries{
		// 		Style: chart.Style{
		// 			Show:        true,             //note; if we set ANY other properties, we must set this to true.
		// 			StrokeColor: drawing.ColorRed, // will supercede defaults
		// 			// FillColor:   drawing.ColorBlue.WithAlpha(64), // will supercede defaults
		// 			// StrokeDashArray: []float64{5.0, 5.0},
		// 		},
		// 		Name:    "fastlane",
		// 		XValues: []float64{50, 75, 90, 95, 99, 100},
		// 		YValues: []float64{1.0, 5.0, 10.0, 150.0, 500.0, 1000.0},
		// 	},
		// },
	}

	//note we have to do this as a separate step because we need a reference to graph
	graph.Elements = []chart.Renderable{
		chart.Legend(&graph),
	}

	// graph.Render(chart.PNG, res)
	buffer := bytes.NewBuffer([]byte{})
	err := graph.Render(chart.PNG, buffer)
	if err != nil {
		fmt.Println(err)
	}

	err = ioutil.WriteFile(*image, buffer.Bytes(), 0644)
	if err != nil {
		fmt.Println(err)
	}
}

func postToImgur(filename string) (string, error) {
	output, err := goImgur.Upload("results.png", "70ff50b8dfc3a53")
	if err != nil {
		return "", err
	}

	var imgurResult map[string]*json.RawMessage
	err = json.Unmarshal([]byte(*output), &imgurResult)
	if err != nil {
		return "", err
	}

	var data map[string]*json.RawMessage
	err = json.Unmarshal(*imgurResult["data"], &data)
	if err != nil {
		return "", err
	}

	var link string
	err = json.Unmarshal(*data["link"], &link)
	if err != nil {
		return "", err
	}
	return link, nil
}
