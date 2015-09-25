// Command getstats polls a expvar endpoint and dumps collectd values for
// memory information and exphttp and exprpcs data entries.
package main

import (
	"flag"
	"fmt"
	"os"
	"time"

	"github.com/pbnjay/exphttp"
)

var (
	instanceName  = flag.String("i", "", "instance name to use")
	hostName      = flag.String("h", "", "hostname to use")
	baseURL       = flag.String("u", "http://127.0.0.1:9000/debug/vars", "expvar URL to use")
	watchInterval = flag.Duration("w", time.Second*10, "watch interval to use")
)

func main() {
	*hostName, _ = os.Hostname()
	flag.Parse()

	opts := fmt.Sprintf("interval=%d", int(watchInterval.Seconds()))

	if *instanceName != "" {
		*instanceName = "-" + *instanceName
	}

	poller := exphttp.ExpPoller{
		BaseURL: *baseURL,
	}

	poller.RecordFunc = func(key string, value interface{}) {
		fmt.Printf("PUTVAL %s/%s%s/gauge-%s %s %d:%v\n",
			*hostName, poller.PluginName, *instanceName,
			key, opts, poller.FetchTime.UTC().Unix(), value)
	}

	for {
		if poller.Fetch() == nil {
			poller.MemStats()
			poller.HTTPStats()
			poller.RPCStats()
		}

		time.Sleep(*watchInterval)
	}
}
