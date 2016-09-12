package main

import (
	"encoding/json"
	"fmt"
	"image/color"
	"os/exec"
	"strconv"
	"strings"

	"github.com/dlion/goImgur"
	kingpin "gopkg.in/alecthomas/kingpin.v2"

	"github.com/gonum/plot"
	"github.com/gonum/plot/plotter"
	"github.com/gonum/plot/vg"
	"github.com/gonum/plot/vg/draw"
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
	passes      = kingpin.Flag("passes", "Number of passes to run a benchmark to eliminate anomalies.").Default("1").Uint16()

	colors = []color.RGBA{
		color.RGBA{R: 255, G: 0, B: 0, A: 255},
		color.RGBA{R: 0, G: 0, B: 255, A: 255},
		color.RGBA{R: 0, G: 255, B: 0, A: 255},
		color.RGBA{R: 0, G: 0, B: 0, A: 255},
	}

	shapes = []draw.GlyphDrawer{
		draw.SquareGlyph{},
		draw.CircleGlyph{},
		draw.CrossGlyph{},
		draw.PyramidGlyph{},
	}
)

func main() {
	kingpin.Parse()

	p, err := plot.New()
	if err != nil {
		panic(err)
	}
	p.Title.Text = fmt.Sprintf("connections: %d, pipelined: %d", *connections, *pipelined)
	p.BackgroundColor = color.White

	p.X.Label.Text = "percentile"
	p.X.Scale = plot.LogScale{}
	p.Y.Label.Text = "latency (milliseconds)"
	// Use a custom tick marker interface implementation with the Ticks function,
	// that computes the default tick marks and re-labels the major ticks with commas.
	p.Y.Tick.Marker = commaTicks{}
	p.Y.Scale = plot.LogScale{}

	// Draw a grid behind the data
	p.Add(plotter.NewGrid())

	// var series []chart.Series
	addresses := strings.Split(*hosts, ",")

	for index, address := range addresses {
		var offset = 0
		var name string
		var results string

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

		results, err = runBenchmark(host, port)
		if err != nil {
			fmt.Println(err)
		}

		points := parseResults(name, results)

		// Make a line plotter with points and set its style.
		var lpLine *plotter.Line
		//var lpPoints *plotter.Scatter
		lpLine, _, err = plotter.NewLinePoints(points)
		if err != nil {
			panic(err)
		}

		lpLine.Color = colors[index]
		lpLine.LineStyle.Dashes = []vg.Length{vg.Points(5), vg.Points(5)}
		// lpPoints.Shape = shapes[index]
		//lpPoints.Color = colors2[index]

		// Add the plotters to the plot, with a legend entry for each
		p.Add(lpLine)
		p.Legend.Add(name, lpLine)

		if len(addresses) > 1 && index < len(addresses)-1 {
			fmt.Printf("Sleeping between runs for 5 seconds\n")
			//time.Sleep(time.Second * 5)
		}
	}

	// Save the plot to a PNG file.
	if err = p.Save(8*vg.Inch, 4*vg.Inch, "results.png"); err != nil {
		panic(err)
	}

	url, err := postToImgur("results.png")
	if err != nil {
		fmt.Println(err)
	}
	fmt.Printf("![](%s)\n", url)
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

func parseResults(name string, results string) plotter.XYs {
	startResults := false
	endResults := false

	lines := strings.Split(results, "\n")
	points := make(plotter.XYs, len(lines)-4)

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

			points[i].X = percentile
			points[i].Y = latency
		}
	}

	nameString := fmt.Sprintf("%s max: %s at %s", name, lines[len(lines)-5], lines[len(lines)-4])
	fmt.Println(nameString)

	return points
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

type commaTicks struct{}

// Ticks computes the default tick marks, but inserts commas
// into the labels for the major tick marks.
func (commaTicks) Ticks(min, max float64) []plot.Tick {
	tks := plot.DefaultTicks{}.Ticks(min, max)
	for i, t := range tks {
		if t.Label == "" { // Skip minor ticks, they are fine.
			continue
		}
		tks[i].Label = addCommas(t.Label)
	}
	return tks
}

// AddCommas adds commas after every 3 characters from right to left.
// NOTE: This function is a quick hack, it doesn't work with decimal
// points, and may have a bunch of other problems.
func addCommas(s string) string {
	rev := ""
	n := 0
	for i := len(s) - 1; i >= 0; i-- {
		rev += string(s[i])
		n++
		if n%3 == 0 {
			rev += ","
		}
	}
	s = ""
	for i := len(rev) - 1; i >= 0; i-- {
		s += string(rev[i])
	}
	return s
}
