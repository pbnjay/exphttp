package exphttp

import (
	"encoding/json"
	"fmt"
	"net/http"
	"runtime"
	"strings"
	"time"
)

type ExpPoller struct {
	PluginName string // plugin
	BaseURL    string
	FetchTime  time.Time
	Vars       map[string]json.RawMessage

	RecordFunc func(key string, val interface{})
}

func (x *ExpPoller) Fetch() error {
	if x.RecordFunc == nil {
		x.RecordFunc = func(k string, v interface{}) {
			DefaultRecordFunc(x, k, v)
		}
	}
	resp, err := http.Get(x.BaseURL)
	if err != nil {
		return err
	}

	x.FetchTime = time.Now()
	return json.NewDecoder(resp.Body).Decode(&x.Vars)
}

func DefaultRecordFunc(x *ExpPoller, key string, value interface{}) {
	fmt.Println(x.FetchTime, x.PluginName, key, value)
}

func (x *ExpPoller) MemStats() error {
	var r runtime.MemStats

	err := json.Unmarshal(x.Vars["memstats"], &r)
	if err != nil {
		return err
	}

	x.PluginName = "memstats"
	x.RecordFunc("alloc", r.Alloc)
	x.RecordFunc("total", r.TotalAlloc)
	x.RecordFunc("sys", r.Sys)
	x.RecordFunc("lookups", r.Lookups)
	x.RecordFunc("mallocs", r.Mallocs)
	x.RecordFunc("frees", r.Frees)

	x.RecordFunc("heap.alloc", r.HeapAlloc)
	x.RecordFunc("heap.sys", r.HeapSys)
	x.RecordFunc("heap.idle", r.HeapIdle)
	x.RecordFunc("heap.inuse", r.HeapInuse)
	x.RecordFunc("heap.released", r.HeapReleased)
	x.RecordFunc("heap.objects", r.HeapObjects)

	x.RecordFunc("stack.inuse", r.StackInuse)
	x.RecordFunc("stack.sys", r.StackSys)
	x.RecordFunc("mspan.inuse", r.MSpanInuse)
	x.RecordFunc("mspan.sys", r.MSpanSys)
	x.RecordFunc("mcache.inuse", r.MCacheInuse)
	x.RecordFunc("mcache.sys", r.MCacheSys)

	x.RecordFunc("gc.count", r.NumGC)
	x.RecordFunc("gc.total_pause_ns", r.PauseTotalNs)

	// calculate average of last 256 GC pauses
	n := uint64(r.NumGC)
	if n > 256 {
		n = 256
	}
	var avg, max uint64
	for i := uint64(0); i < n; i++ {
		avg += r.PauseNs[i] / n
		if r.PauseNs[i] > max {
			max = r.PauseNs[i]
		}
	}
	x.RecordFunc("gc.avg_pause_ns", avg)
	x.RecordFunc("gc.max_pause_ns", max)

	return nil
}

func (x *ExpPoller) HTTPStats() error {
	if _, f := x.Vars["exphttp"]; !f {
		return nil
	}

	var h map[string]int
	err := json.Unmarshal(x.Vars["exphttp"], &h)
	if err != nil {
		return err
	}
	x.PluginName = "http"
	for endpoint := range h {
		var r map[string]float64

		err = json.Unmarshal(x.Vars[endpoint], &r)
		if err != nil {
			return err
		}

		for key, val := range r {
			x.RecordFunc(endpoint+"."+key, val)
			if strings.HasSuffix(key, ".total_ns") {
				k2 := strings.TrimSuffix(key, ".total_ns")
				x.RecordFunc(endpoint+"."+k2+".avg_ns", val/r[k2])
			}
		}

		x.RecordFunc(endpoint+".queue_depth", r["requests"]-r["responses"])
		x.RecordFunc(endpoint+".success_rate", r["responses.200"]*100.0/r["requests"])
		x.RecordFunc(endpoint+".error_rate", (r["responses"]-r["responses.200"])*100.0/r["requests"])
	}
	return nil
}

func (x *ExpPoller) RPCStats() error {
	if _, f := x.Vars["exprpc"]; !f {
		return nil
	}

	var r map[string]float64
	err := json.Unmarshal(x.Vars["exprpc"], &r)
	if err != nil {
		return err
	}

	x.PluginName = "rpc"
	for key, val := range r {
		x.RecordFunc(key, val)
		if strings.HasSuffix(key, ".total_ns") {
			k2 := strings.TrimSuffix(key, ".total_ns")
			x.RecordFunc(k2+".avg_ns", val/r[k2])
		}
	}

	x.RecordFunc("queue_depth", r["requests"]-r["responses"])
	x.RecordFunc("error_rate", r["responses.error"]*100.0/r["requests"])
	x.RecordFunc("success_rate", (r["responses"]-r["responses.error"])*100.0/r["requests"])
	return nil
}
