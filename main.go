package main

import (
	"encoding/json"
	"fmt"
	img "image"
	"image/color"
	"math"
	"os/exec"
	"strconv"
	"strings"
	"time"

	"github.com/disintegration/imaging"
	"github.com/dlion/goImgur"
	kingpin "gopkg.in/alecthomas/kingpin.v2"

	"github.com/gonum/plot"
	"github.com/gonum/plot/plotter"
	"github.com/gonum/plot/plotutil"
	"github.com/gonum/plot/vg"
)

type result struct {
	name          string
	latencyPoints plotter.XYs
	throughput    float64
	max           float64
}

var (
	verbose     = kingpin.Flag("verbose", "Verbose mode.").Short('v').Bool()
	hosts       = kingpin.Flag("hosts", "Host addresses for the target Redis servers to benchmark against.").Short('h').Required().String()
	image       = kingpin.Flag("image", "Where to store the results graph in PNG format.").Default("results.jpg").Short('i').String()
	requests    = kingpin.Flag("requests", "Number of total requests.").Short('r').Default("10000000").Uint32()
	connections = kingpin.Flag("connections", "Number of Redis client connections.").Short('c').Default("128").Uint16()
	pipelined   = kingpin.Flag("pipelined", "Number of pipelined requests per connection.").Short('p').Default("128").Uint16()
	duration    = kingpin.Flag("duration", "Duration in seconds to run the benchmarks.").Default("10").Uint16()
	sleep       = kingpin.Flag("sleep", "Duration in seconds to sleep between benchmarks.").Default("0").Uint16()
)

func main() {
	kingpin.Parse()

	var results []result
	addresses := strings.Split(*hosts, ",")

	startingTime := time.Now()

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

		if *verbose {
			fmt.Printf("Running benchmark against %s on %s:%s\n", name, host, port)
		}

		output, err := runBenchmark(host, port)
		if err != nil {
			fmt.Println(err)
		} else {
			if *verbose {
				fmt.Println(string(output))
			}

			r := parseResults(name, output)
			results = append(results, r)

			if len(addresses) > 1 && index < len(addresses)-1 {
				time.Sleep(time.Duration(*sleep) * time.Second)
			}
		}
	}

	generateLatencyDistributionGraph(results)
	generateThroughputGraph(results)
	generateMaxLatencyGraph(results)
	combineImages()

	url, err := postToImgur(*image)
	if err != nil {
		fmt.Println(err)
	}

	duration := time.Now().Sub(startingTime)
	fmt.Printf("![%s\t%d/%d](%s)\n", duration, *connections, *pipelined, url)
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
		strconv.FormatUint(uint64(*pipelined), 10),
		"-D",
		strconv.FormatUint(uint64(*duration), 10))

	output, err := cmd.Output()
	return string(output), err
}

func parseResults(name string, results string) result {
	startResults := false
	endResults := false
	var lastResult float64

	lines := strings.Split(results, "\n")
	points := make(plotter.XYs, 0)
	var r result

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
			latency = latency / 1000.00
			lastResult = latency

			if percentile > 0.0 && latency > 0.0 {
				points = append(points, struct{ X, Y float64 }{percentile, latency})
			}
		}
	}

	// nameString := fmt.Sprintf("%s max: %s at %s", name, lines[len(lines)-5], lines[len(lines)-4])

	r.name = name
	r.latencyPoints = points
	throughputStringParts := strings.Split(lines[len(lines)-4], " ")
	r.throughput, _ = strconv.ParseFloat(throughputStringParts[0], 64)
	r.max = lastResult

	return r
}

func generateLatencyDistributionGraph(results []result) {
	p, err := plot.New()
	if err != nil {
		panic(err)
	}
	p.Title.Text = fmt.Sprintf("connections: %d, pipelined: %d", *connections, *pipelined)
	p.Legend.Top = true
	p.Legend.Left = true
	// p.BackgroundColor = color.Black
	// p.Legend.TextStyle.Color = color.White
	// p.X.Label.Color = color.White
	// p.Y.Label.Color = color.White
	// p.X.Color = color.White
	// p.Y.Color = color.White
	// p.X.Tick.Color = color.White
	// p.Y.Tick.Color = color.White
	// p.X.Tick.Label.Color = color.White
	// p.Y.Tick.Label.Color = color.White

	p.X.Label.Text = "percentile"
	p.Y.Label.Text = "latency (milliseconds)"
	// Use a custom tick marker interface implementation with the Ticks function,
	// that computes the default tick marks and re-labels the major ticks with commas.
	// p.Y.Tick.Marker = commaTicks{}

	// p.X.Scale = plot.LogScale{}
	// p.Y.Scale = plot.LogScale{}
	p.X.Min = 0.0
	p.X.Max = 100.0
	p.X.Scale = ReverseLogScale{}

	// Draw a grid behind the data
	p.Add(plotter.NewGrid())

	for index, r := range results {
		// Make a line plotter with points and set its style.
		var lpLine *plotter.Line
		// var lpPoints *plotter.Scatter
		// lpLine, lpPoints, err = plotter.NewLinePoints(r.latencyPoints)
		lpLine, _, err = plotter.NewLinePoints(r.latencyPoints)
		if err != nil {
			panic(err)
		}
		lpLine.Color = plotutil.Color(index)
		// lpLine.LineStyle.Dashes = []vg.Length{vg.Points(5), vg.Points(5)}

		// lpPoints.Shape = shapes[index]
		// lpPoints.Color = colors[index]

		// Add the plotters to the plot, with a legend entry for each
		p.Add(lpLine)
		p.Legend.Add(r.name, lpLine)

	}

	// Save the plot to a PNG file.
	if err = p.Save(8*vg.Inch, 4*vg.Inch, "results_latency.png"); err != nil {
		panic(err)
	}
}

func generateThroughputGraph(results []result) {
	p, err := plot.New()
	if err != nil {
		panic(err)
	}
	p.Title.Text = "throughput"
	p.Y.Label.Text = "operations/second (millions)"
	p.Legend.Top = true

	offsetPadding := -100.0

	for index, r := range results {
		value := plotter.Values{r.throughput / 1000000}
		var bars *plotter.BarChart
		width := vg.Points(40)
		offset := vg.Points(float64(40*(index+1)) + offsetPadding)

		bars, err = plotter.NewBarChart(value, width)
		if err != nil {
			panic(err)
		}
		bars.LineStyle.Width = vg.Length(0)
		bars.Color = plotutil.Color(index)
		bars.Offset = offset

		p.Add(bars)
		p.Legend.Add(r.name, bars)
	}
	p.NominalX("")

	if err = p.Save(3.5*vg.Inch, 4*vg.Inch, "results_throughput.png"); err != nil {
		panic(err)
	}
}

func generateMaxLatencyGraph(results []result) {
	p, err := plot.New()
	if err != nil {
		panic(err)
	}
	p.Title.Text = "max latency"
	p.Y.Label.Text = "milliseconds"
	p.Legend.Top = true

	offsetPadding := -100.0

	for index, r := range results {
		value := plotter.Values{r.max}
		var bars *plotter.BarChart
		width := vg.Points(40)
		offset := vg.Points(float64(40*(index+1)) + offsetPadding)

		bars, err = plotter.NewBarChart(value, width)
		if err != nil {
			panic(err)
		}
		bars.LineStyle.Width = vg.Length(0)
		bars.Color = plotutil.Color(index)
		bars.Offset = offset

		p.Add(bars)
		p.Legend.Add(r.name, bars)
	}
	p.NominalX("")

	if err = p.Save(3.5*vg.Inch, 4*vg.Inch, "results_max.png"); err != nil {
		panic(err)
	}
}

func postToImgur(filename string) (string, error) {
	output, err := goImgur.Upload(filename, "70ff50b8dfc3a53")
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

func combineImages() {
	// Input files
	files := []string{"results_latency.png", "results_throughput.png", "results_max.png"}

	// Load images
	var images []img.Image
	var width int
	var height int
	xPadding := 20
	yPadding := 20

	for _, file := range files {
		imgFile, err := imaging.Open(file)
		if err != nil {
			panic(err)
		}
		images = append(images, imgFile)
		width += imgFile.Bounds().Dx() + xPadding
		height = int(math.Max(float64(height), float64(imgFile.Bounds().Dy())))
	}

	width += xPadding * 2
	height += yPadding

	// Create a new blank image
	dst := imaging.New(width, height, color.NRGBA{255, 255, 255, 255})

	// paste thumbnails into the new image side by side
	x := xPadding
	for _, imgFile := range images {
		dst = imaging.Paste(dst, imgFile, img.Pt(x, yPadding))
		x += imgFile.Bounds().Dx() + xPadding
	}

	// save the combined image to file
	err := imaging.Save(dst, "results.jpg")
	if err != nil {
		panic(err)
	}
}

// CustomLogScale can be used as the value of an Axis.Scale function to
// set the axis to a log scale.
type CustomLogScale struct{}

// var _ Normalizer = CustomLogScale{}

// Normalize returns the fractional logarithmic distance of
// x between min and max.
func (CustomLogScale) Normalize(min, max, x float64) float64 {
	logMin := logscale(min)
	return (logscale(x) - logMin) / (logscale(max) - logMin)
}

func logscale(x float64) float64 {
	if x <= 0 {
		panic("Values must be greater than 0 for a log scale.")
	}
	return math.Log(x)
}

// ReverseLogScale can be used as the value of an Axis.Scale function to
// set the axis to a log scale.
type ReverseLogScale struct{}

// Normalize returns the fractional logarithmic distance of
// x between min and max.
func (ReverseLogScale) Normalize(min, max, x float64) float64 {
	logMin := math.Log(min)
	return 1 - ((math.Log(max-x) - logMin) / (math.Log(max) - logMin))
}
