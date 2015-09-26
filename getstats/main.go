// Command getstats polls a expvar endpoint and dumps values for collectd
// including memstats, exphttp, exprpcs data entries.
//
// Usage is straightforward with the collectd `exec` plugin:
//    https://collectd.org/documentation/manpages/collectd-exec.5.shtml
//
// Loading an example exec.conf:
//     LoadPlugin exec
//     <Plugin exec>
//        Exec "user:group" "/path/to/getstats" "-h" "prod1" "-i main" "-u" "http://127.0.0.1:3000/debug/vars"
//     </Plugin>
//
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
