package main

import (
	"flag"
	"fmt"
	"math"
	"net/url"
	"os"
	"time"

	"github.com/briandowns/spinner"
	"github.com/dustin/go-humanize"
	"github.com/glentiki/hdrhistogram"
	"github.com/jbenet/goprocess"
	"github.com/olekukonko/tablewriter"
	"github.com/ttacon/chalk"
	"github.com/valyala/fasthttp"
)

type resp struct {
	status  int
	latency int64
	size    int
}

func main() {
	uri := flag.String("uri", "", "The uri to benchmark against. (Required)")
	clients := flag.Int("connections", 10, "The number of connections to open to the server.")
	pipeliningFactor := flag.Int("pipelining", 1, "The number of pipelined requests to use.")
	runtime := flag.Int("duration", 10, "The number of seconds to run the autocannnon.")
	timeout := flag.Int("timeout", 10, "The number of seconds before timing out on a request.")
	debug := flag.Bool("debug", false, "A utility debug flag.")
	flag.Parse()

	if *uri == "" {
		flag.PrintDefaults()
		os.Exit(1)
	}

	fmt.Println(fmt.Sprintf("running %vs test @ %v", *runtime, *uri))
	fmt.Println(fmt.Sprintf("%v connections with %v pipelining factor.", *clients, *pipeliningFactor))

	proc := goprocess.Background()

	respChan, errChan := runClients(proc, *clients, *pipeliningFactor, time.Second*time.Duration(*timeout), *uri)

	latencies := hdrhistogram.New(1, 10000, 5)
	requests := hdrhistogram.New(1, 1000000, 5)
	throughput := hdrhistogram.New(1, 100000000000, 5)

	var bytes int64 = 0
	var totalBytes int64 = 0
	var respCounter int64 = 0
	var totalResp int64 = 0

	resp2xx := 0
	respN2xx := 0

	errors := 0
	timeouts := 0

	ticker := time.NewTicker(time.Second)
	runTimeout := time.NewTimer(time.Second * time.Duration(*runtime))

	spin := spinner.New(spinner.CharSets[14], 100*time.Millisecond)
	spin.Suffix = " Running Autocannon..."
	spin.Start()

	for {
		select {
		case err := <-errChan:
			errors++
			if *debug {
				fmt.Printf("there was an error: %s\n", err.Error())
			}
			if err == fasthttp.ErrTimeout {
				timeouts++
			}
		case res := <-respChan:
			s := int64(res.size)
			bytes += s
			totalBytes += s
			respCounter++

			totalResp++
			if res.status >= 200 && res.status < 300 {
				latencies.RecordValue(int64(res.latency))
				resp2xx++
			} else {
				respN2xx++
			}

		case <-ticker.C:
			requests.RecordValue(respCounter)
			respCounter = 0
			throughput.RecordValue(bytes)
			bytes = 0
			// fmt.Println("done ticking")
		case <-runTimeout.C:
			spin.Stop()

			fmt.Println("")
			fmt.Println("")
			shortLatency := tablewriter.NewWriter(os.Stdout)
			shortLatency.SetRowSeparator("-")
			shortLatency.SetHeader([]string{
				"Stat",
				"2.5%",
				"50%",
				"97.5%",
				"99%",
				"Avg",
				"Stdev",
				"Max",
			})
			shortLatency.SetHeaderColor(tablewriter.Colors{tablewriter.Bold, tablewriter.FgCyanColor},
				tablewriter.Colors{tablewriter.Bold, tablewriter.FgCyanColor},
				tablewriter.Colors{tablewriter.Bold, tablewriter.FgCyanColor},
				tablewriter.Colors{tablewriter.Bold, tablewriter.FgCyanColor},
				tablewriter.Colors{tablewriter.Bold, tablewriter.FgCyanColor},
				tablewriter.Colors{tablewriter.Bold, tablewriter.FgCyanColor},
				tablewriter.Colors{tablewriter.Bold, tablewriter.FgCyanColor},
				tablewriter.Colors{tablewriter.Bold, tablewriter.FgCyanColor})
			shortLatency.Append([]string{
				chalk.Bold.TextStyle("Latency"),
				fmt.Sprintf("%v ms", latencies.ValueAtPercentile(2.5)),
				fmt.Sprintf("%v ms", latencies.ValueAtPercentile(50)),
				fmt.Sprintf("%v ms", latencies.ValueAtPercentile(97.5)),
				fmt.Sprintf("%v ms", latencies.ValueAtPercentile(99)),
				fmt.Sprintf("%.2f ms", latencies.Mean()),
				fmt.Sprintf("%.2f ms", latencies.StdDev()),
				fmt.Sprintf("%v ms", latencies.Max()),
			})
			shortLatency.Render()
			fmt.Println("")
			fmt.Println("")

			requestsTable := tablewriter.NewWriter(os.Stdout)
			requestsTable.SetRowSeparator("-")
			requestsTable.SetHeader([]string{
				"Stat",
				"1%",
				"2.5%",
				"50%",
				"97.5%",
				"Avg",
				"Stdev",
				"Min",
			})
			requestsTable.SetHeaderColor(tablewriter.Colors{tablewriter.Bold, tablewriter.FgCyanColor},
				tablewriter.Colors{tablewriter.Bold, tablewriter.FgCyanColor},
				tablewriter.Colors{tablewriter.Bold, tablewriter.FgCyanColor},
				tablewriter.Colors{tablewriter.Bold, tablewriter.FgCyanColor},
				tablewriter.Colors{tablewriter.Bold, tablewriter.FgCyanColor},
				tablewriter.Colors{tablewriter.Bold, tablewriter.FgCyanColor},
				tablewriter.Colors{tablewriter.Bold, tablewriter.FgCyanColor},
				tablewriter.Colors{tablewriter.Bold, tablewriter.FgCyanColor})
			requestsTable.Append([]string{
				chalk.Bold.TextStyle("Req/Sec"),
				fmt.Sprintf("%v", requests.ValueAtPercentile(1)),
				fmt.Sprintf("%v", requests.ValueAtPercentile(2.5)),
				fmt.Sprintf("%v", requests.ValueAtPercentile(50)),
				fmt.Sprintf("%v", requests.ValueAtPercentile(97.5)),
				fmt.Sprintf("%.2f", requests.Mean()),
				fmt.Sprintf("%.2f", requests.StdDev()),
				fmt.Sprintf("%v", requests.Min()),
			})
			requestsTable.Append([]string{
				chalk.Bold.TextStyle("Bytes/Sec"),
				fmt.Sprintf("%v", humanize.Bytes(uint64(throughput.ValueAtPercentile(1)))),
				fmt.Sprintf("%v", humanize.Bytes(uint64(throughput.ValueAtPercentile(2.5)))),
				fmt.Sprintf("%v", humanize.Bytes(uint64(throughput.ValueAtPercentile(50)))),
				fmt.Sprintf("%v", humanize.Bytes(uint64(throughput.ValueAtPercentile(97.5)))),
				fmt.Sprintf("%v", humanize.Bytes(uint64(throughput.Mean()))),
				fmt.Sprintf("%v", humanize.Bytes(uint64(throughput.StdDev()))),
				fmt.Sprintf("%v", humanize.Bytes(uint64(throughput.Min()))),
			})
			requestsTable.Render()

			fmt.Println("")
			fmt.Println("Req/Bytes counts sampled once per second.")
			fmt.Println("")
			fmt.Println("")
			fmt.Println(fmt.Sprintf("%v 2xx responses, %v non 2xx responses.", resp2xx, respN2xx))
			fmt.Println(fmt.Sprintf("%v total requests in %v seconds, %s read.", formatBigNum(float64(totalResp)), *runtime, humanize.Bytes(uint64(totalBytes))))
			if errors > 0 {
				fmt.Println(fmt.Sprintf("%v total errors (%v timeouts).", formatBigNum(float64(errors)), formatBigNum(float64(timeouts))))
			}
			fmt.Println("Done!")

			os.Exit(0)
		}
	}
}

func formatBigNum(i float64) string {
	if i < 1000 {
		return fmt.Sprintf("%.0f", i)
	}
	return fmt.Sprintf("%.0fk", math.Round(i/1000))
}

func runClients(ctx goprocess.Process, clients int, pipeliningFactor int, timeout time.Duration, uri string) (<-chan *resp, <-chan error) {
	respChan := make(chan *resp, 2*clients*pipeliningFactor)
	errChan := make(chan error, 2*clients*pipeliningFactor)
	u, _ := url.Parse(uri)

	for i := 0; i < clients; i++ {
		c := fasthttp.PipelineClient{
			Addr:               getAddr(u),
			IsTLS:              u.Scheme == "https",
			MaxPendingRequests: pipeliningFactor,
		}

		for j := 0; j < pipeliningFactor; j++ {
			go func() {
				req := fasthttp.AcquireRequest()
				req.SetBody([]byte("hello, world!"))
				req.SetRequestURI(uri)

				res := fasthttp.AcquireResponse()

				for {
					startTime := time.Now()
					if err := c.DoTimeout(req, res, timeout); err != nil {
						errChan <- err
					} else {
						size := len(res.Body()) + 2
						res.Header.VisitAll(func(key, value []byte) {
							size += len(key) + len(value) + 2
						})
						respChan <- &resp{
							status:  res.Header.StatusCode(),
							latency: time.Now().Sub(startTime).Milliseconds(),
							size:    size,
						}
						res.Reset()
					}
				}
			}()
		}
	}
	return respChan, errChan
}

// getAddr returns the address from a URL, including the port if it's not empty.
// So it can return hostname:port or simply hostname
func getAddr(u *url.URL) string {
	if u.Port() == "" {
		return u.Hostname()
	} else {
		return fmt.Sprintf("%v:%v", u.Hostname(), u.Port())
	}
}
