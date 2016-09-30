package main

import (
	"encoding/json"
	"fmt"
	img "image"
	"image/color"
	"math"
	"os/exec"
	"runtime"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/disintegration/imaging"
	"github.com/dlion/goImgur"
	"github.com/garyburd/redigo/redis"
	kingpin "gopkg.in/alecthomas/kingpin.v2"

	"github.com/gonum/plot"
	"github.com/gonum/plot/plotter"
	"github.com/gonum/plot/plotutil"
	"github.com/gonum/plot/vg"
	"github.com/gonum/plot/vg/draw"
)

type result struct {
	name          string
	latencyPoints plotter.XYs
	throughput    float64
	max           float64
}

var (
	verbose     = kingpin.Flag("verbose", "Verbose mode.").Short('v').Bool()
	hosts       = kingpin.Flag("hosts", "Host addresses for the target Redis servers to benchmark against.").Required().String()
	image       = kingpin.Flag("image", "Where to store the results graph in PNG format.").Default("results.jpg").String()
	threads     = kingpin.Flag("threads", "Number of CPU threads to use.").Default(fmt.Sprintf("%d", runtime.NumCPU())).String()
	connections = kingpin.Flag("connections", "Number of Redis client connections.").Default("32").Uint16()
	pipelined   = kingpin.Flag("pipelined", "Number of pipelined requests per connection.").Default("1").Uint16()
	sleep       = kingpin.Flag("sleep", "Duration in seconds to sleep between benchmarks.").Default("0").Uint16()
	duration    = kingpin.Flag("duration", "Duration in seconds to run benchmark stages.").Default("10").Uint16()

	shapes = []draw.GlyphDrawer{
		draw.SquareGlyph{},
		draw.CircleGlyph{},
		draw.CrossGlyph{},
		draw.PyramidGlyph{},
	}

	httpBasePort = 8080
)

func main() {
	kingpin.Parse()

	pools = make(map[int64]*redis.Pool)

	startHTTPServers()
	time.Sleep(time.Duration(*sleep) * time.Second)
	benchmark()
}

func startHTTPServers() {
	addresses := strings.Split(*hosts, ",")
	httpPort := httpBasePort

	for _, address := range addresses {
		httpPort++
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

		listenAddress := fmt.Sprintf("%s:%s", host, port)
		if *verbose {
			fmt.Printf("starting http server for %s listening on %d\n", name, httpPort)
		}
		go startHTTPServer(listenAddress, int(*connections), httpPort)
	}
}

func benchmark() {
	start := time.Now()

	httpPort := httpBasePort
	var results []*result
	addresses := strings.Split(*hosts, ",")

	for index, address := range addresses {
		var offset = 0
		var name string
		httpPort++

		hostParts := strings.Split(address, ":")
		if len(hostParts) > 2 {
			offset = 1
			name = hostParts[0]
		} else {
			name = hostParts[0] + " " + hostParts[1]
		}
		host := hostParts[0+offset]
		port, _ := strconv.ParseInt(hostParts[1+offset], 10, 32)

		r := &result{}
		r.name = name

		throughputOutput, err := runWrkThroughputBenchmark(name, host, int(port), httpPort)
		if err != nil {
			fmt.Println(err)
			fmt.Println(throughputOutput)
		} else {
			if *verbose {
				fmt.Println(string(throughputOutput))
			}
			parseWrkThroughputResults(name, throughputOutput, r)

		}

		latencyOutput, err := runWrkLatencyBenchmark(name, host, int(port), httpPort, int(r.throughput))
		if err != nil {
			fmt.Println(err)
			fmt.Println(latencyOutput)
		} else {
			if *verbose {
				fmt.Println(string(latencyOutput))
			}

			parseWrkLatencyResults(name, latencyOutput, r)
			results = append(results, r)

			if len(addresses) > 1 && index < len(addresses)-1 && *sleep > 0 {
				time.Sleep(time.Duration(*sleep) * time.Second)
			}
		}
	}
	elapsed := time.Since(start)

	generateLatencyDistributionGraph(results)
	generateThroughputGraph(results)
	generateMaxLatencyGraph(results)
	combineImages()

	url, err := postToImgur(*image)
	if err != nil {
		fmt.Println(err)
	}
	fmt.Printf("%d/%d took %s: ![](%s)\n", *connections, *pipelined, elapsed, url)
}

func runWrkThroughputBenchmark(name string, host string, redisPort int, httpPort int) (string, error) {
	connectionsArg := strconv.FormatUint(uint64(*connections), 10)
	pipelinedArg := strconv.FormatUint(uint64(*pipelined), 10)

	if *verbose {
		fmt.Printf("Running benchmark for %s on %s:%d\n\tConnections:\t%s\n\tPipelined:\t%s\n", name, host, redisPort, connectionsArg, pipelinedArg)
	}

	command := "./benchmark/wrk"
	args := []string{
		"--latency",
		"--script",
		"./benchmark/set_random.lua",
		"--threads",
		*threads,
		"--connections",
		connectionsArg,
		"--duration",
		fmt.Sprintf("%ds", *duration),
		fmt.Sprintf("http://localhost:%d", httpPort),
		"--",
		pipelinedArg,
	}

	if *verbose {
		fmt.Println(command + " " + strings.Join(args, " "))
	}
	cmd := exec.Command(command, args...)

	output, err := cmd.CombinedOutput()
	return string(output), err
}

func runWrkLatencyBenchmark(name string, host string, redisPort int, httpPort int, rate int) (string, error) {
	connectionsArg := strconv.FormatUint(uint64(*connections), 10)
	pipelinedArg := strconv.FormatUint(uint64(*pipelined), 10)

	if *verbose {
		fmt.Printf("Running benchmark for %s on %s:%d\n\tConnections:\t%s\n\tPipelined:\t%s\n", name, host, redisPort, connectionsArg, pipelinedArg)
	}

	command := "./benchmark/wrk2"
	args := []string{
		"--latency",
		"--script",
		"./benchmark/set_random.lua",
		"--threads",
		*threads,
		"--connections",
		connectionsArg,
		"--duration",
		fmt.Sprintf("%ds", *duration),
		"--rate",
		strconv.Itoa(rate),
		fmt.Sprintf("http://localhost:%d", httpPort),
		"--",
		pipelinedArg}

	if *verbose {
		fmt.Println(command + " " + strings.Join(args, " "))
	}
	cmd := exec.Command(command, args...)

	output, err := cmd.CombinedOutput()
	return string(output), err
}

func parseWrkThroughputResults(name string, results string, r *result) {
	lines := strings.Split(results, "\n")

	for i := 0; i < len(lines); i++ {
		line := lines[i]
		if strings.HasPrefix(line, "Requests/sec:") {
			throughputStringParts := strings.Fields(lines[i])
			throughput, _ := strconv.ParseFloat(throughputStringParts[1], 64)
			r.throughput = throughput
			break
		}
	}
}

func parseWrkLatencyResults(name string, results string, r *result) {
	var lastResult float64
	startResults := false
	endResults := false

	entries := make(map[float64]float64)
	lines := strings.Split(results, "\n")

	for i := 0; i < len(lines); i++ {
		line := lines[i]
		if strings.HasPrefix(line, " 50.") && startResults == false {
			startResults = true
		} else if line == "" && startResults == true {
			endResults = true
			break
		}

		if startResults && !endResults {
			lineParts := strings.Fields(line)
			percentileString := strings.Split(lineParts[0], "%")[0]
			latencyString := strings.Split(lineParts[1], "ms")[0]

			percentile, _ := strconv.ParseFloat(percentileString, 64)
			latency, _ := strconv.ParseFloat(latencyString, 64)

			lastResult = latency

			if percentile >= 1 && latency >= 1 {
				entries[percentile] = latency
			}
		}
	}

	points := make(plotter.XYs, len(entries))

	var keys []float64
	for k := range entries {
		keys = append(keys, k)
	}
	sort.Float64s(keys)

	for i, k := range keys {
		points[i].X = k
		points[i].Y = entries[k]
	}

	r.name = name
	r.latencyPoints = points
	r.max = lastResult
}

func generateThroughputGraph(results []*result) {
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

func generateMaxLatencyGraph(results []*result) {
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

func generateLatencyDistributionGraph(results []*result) {
	p, err := plot.New()
	if err != nil {
		panic(err)
	}
	p.Title.Text = fmt.Sprintf("connections: %d, pipelined: %d", *connections, *pipelined)
	p.BackgroundColor = color.White
	p.Legend.Top = true
	p.Legend.Left = true

	p.X.Label.Text = "percentile"
	p.Y.Label.Text = "latency (milliseconds)"
	// Use a custom tick marker interface implementation with the Ticks function,
	// that computes the default tick marks and re-labels the major ticks with commas.
	// p.Y.Tick.Marker = commaTicks{}

	// p.X.Scale = plot.LogScale{}
	// p.Y.Scale = plot.LogScale{}

	// Draw a grid behind the data
	p.Add(plotter.NewGrid())

	for index, r := range results {
		// Make a line plotter with points and set its style.
		var lpLine *plotter.Line
		var lpPoints *plotter.Scatter
		lpLine, lpPoints, err = plotter.NewLinePoints(r.latencyPoints)
		//lpLine, _, err = plotter.NewLinePoints(r.latencyPoints)
		if err != nil {
			panic(err)
		}

		//lpLine.LineStyle.Dashes = []vg.Length{vg.Points(5), vg.Points(5)}
		lpLine.Color = plotutil.Color(index)
		lpPoints.Shape = plotutil.Shape(index)
		// lpPoints.Color = plotutil.Color(index)
		// lpPoints.Shape = plotutil.Shape(index)

		// Add the plotters to the plot, with a legend entry for each
		p.Add(lpLine, lpPoints)
		p.Legend.Add(r.name, lpLine, lpPoints)
	}

	// Save the plot to a PNG file.
	if err = p.Save(8*vg.Inch, 4*vg.Inch, "results_latency.png"); err != nil {
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
