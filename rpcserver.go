package exphttp

import (
	"bufio"
	"encoding/gob"
	"expvar"
	"io"
	"log"
	"net/http"
	"net/rpc"
	"time"
)

var (
	rpcStats *expvar.Map
	reqRate  *RateCounter
	respRate *RateCounter
)

// ExpRPCServer is a wrapped rpc.Server that exposes timing info and request
// stats for all the RPC calls going through a rpc.Server.
type ExpRPCServer struct {
	srv *rpc.Server

	// IntervalLabel is the suffix to append to rate counters.
	IntervalLabel string

	// Interval is the time span to use for rate counters.
	Interval time.Duration

	// Log requests to this logger if non-nil.
	Log *log.Logger

	rates      map[string]*RateCounter
	startTimes map[uint64]time.Time
}

func (w *ExpRPCServer) recordRequest(r *rpc.Request) {
	reqRate.Add(1)
	rpcStats.Add("requests", 1)
	rpcStats.Add("requests."+r.ServiceMethod, 1)
	rc, found := w.rates[r.ServiceMethod]
	if !found {
		rc = NewRateCounter(w.Interval)
		w.rates[r.ServiceMethod] = rc
		rpcStats.Set("requests."+r.ServiceMethod+".per_"+w.IntervalLabel, rc)
	}
	rc.Add(1)
	w.startTimes[r.Seq] = time.Now()
}

func (w *ExpRPCServer) recordResponse(r *rpc.Response) {
	elapsed := time.Now().Sub(w.startTimes[r.Seq]).Nanoseconds()

	respRate.Add(1)
	rpcStats.Add("responses", 1)
	rpcStats.Add("responses.total_ns", elapsed)
	rpcStats.Add("responses."+r.ServiceMethod, 1)
	rpcStats.Add("responses."+r.ServiceMethod+".total_ns", elapsed)
	if r.Error != "" {
		rpcStats.Add("responses.error", 1)
		rpcStats.Add("responses.error.total_ns", elapsed)

		rpcStats.Add("responses."+r.ServiceMethod+".error", 1)
		rpcStats.Add("responses."+r.ServiceMethod+".error.total_ns", elapsed)
	}
	if w.Log != nil {
		w.Log.Println(float64(elapsed)/1000000.0, "ms --", r.ServiceMethod, "--", r.Error)
	}
	delete(w.startTimes, r.Seq)
}

// NewRPCServer creates a new ExpRPCServer wrapping a rpc.Server, publishes a
// new "exprpc" expvar.Map to track it, sets a default IntervalLabel="min" and
// Interval=time.Minute, and sets Log to DefaultLogger.
//
// To register the wrapped RPC endpoint using the same protocol/endpoint as
// the default rpc.HandleHTTP() method, use:
//
//     expServer := exphttp.NewRPCServer(rpc.DefaultServer)
//     http.HandleFunc("/_goRPC_", expServer.HandleHTTP)
//
func NewRPCServer(srv *rpc.Server) *ExpRPCServer {
	if rpcStats == nil {
		rpcStats = expvar.NewMap("exprpc")
		reqRate = NewRateCounter(time.Minute)
		respRate = NewRateCounter(time.Minute)
		rpcStats.Set("requests.per_min", reqRate)
		rpcStats.Set("responses.per_min", respRate)
	}

	e := &ExpRPCServer{
		srv:           srv,
		IntervalLabel: "min",
		Interval:      time.Minute,
		Log:           DefaultLogger,

		rates:      make(map[string]*RateCounter),
		startTimes: make(map[uint64]time.Time),
	}

	return e
}

////////////////////////////
// below this line copied over from unexported stdlib methods and minimally tweaked

type gobServerCodec struct {
	exp *ExpRPCServer

	rwc    io.ReadWriteCloser
	dec    *gob.Decoder
	enc    *gob.Encoder
	encBuf *bufio.Writer
	closed bool
}

func (c *gobServerCodec) ReadRequestHeader(r *rpc.Request) error {
	err := c.dec.Decode(r)
	c.exp.recordRequest(r)
	return err
}

func (c *gobServerCodec) ReadRequestBody(body interface{}) error {
	return c.dec.Decode(body)
}

func (c *gobServerCodec) WriteResponse(r *rpc.Response, body interface{}) (err error) {
	c.exp.recordResponse(r)

	if err = c.enc.Encode(r); err != nil {
		if c.encBuf.Flush() == nil {
			// Gob couldn't encode the header. Should not happen, so if it does,
			// shut down the connection to signal that the connection is broken.
			log.Println("rpc: gob error encoding response:", err)
			c.Close()
		}
		return
	}
	if err = c.enc.Encode(body); err != nil {
		if c.encBuf.Flush() == nil {
			// Was a gob problem encoding the body but the header has been written.
			// Shut down the connection to signal that the connection is broken.
			log.Println("rpc: gob error encoding body:", err)
			c.Close()
		}
		return
	}
	return c.encBuf.Flush()
}

func (c *gobServerCodec) Close() error {
	if c.closed {
		// Only call c.rwc.Close once; otherwise the semantics are undefined.
		return nil
	}
	c.closed = true
	return c.rwc.Close()
}

// HandleHTTP implements an http.HandlerFunc that answers RPC requests, and
// tracks timing info via expvars. It's basically the same as the default
// rpc.HandleHTTP methods, only you have to register this one manually.
//
// For example, to replace this code:
//     rpc.Register(myService)
//     rpc.HandleHTTP()
//
// Use this sequence instead:
//     rpc.Register(myService)
//     expServer := exphttp.NewRPCServer(rpc.DefaultServer)
//     http.HandleFunc("/_goRPC_", expServer.HandleFunc)
//
func (x *ExpRPCServer) HandleFunc(w http.ResponseWriter, req *http.Request) {
	if req.Method != "CONNECT" {
		w.Header().Set("Content-Type", "text/plain; charset=utf-8")
		w.WriteHeader(http.StatusMethodNotAllowed)
		io.WriteString(w, "405 must CONNECT\n")
		return
	}
	conn, _, err := w.(http.Hijacker).Hijack()
	if err != nil {
		log.Print("rpc hijacking ", req.RemoteAddr, ": ", err.Error())
		return
	}
	io.WriteString(conn, "HTTP/1.0 200 Connected to Go RPC\n\n")

	buf := bufio.NewWriter(conn)
	codec := &gobServerCodec{
		exp:    x,
		rwc:    conn,
		dec:    gob.NewDecoder(conn),
		enc:    gob.NewEncoder(buf),
		encBuf: buf,
	}
	x.srv.ServeCodec(codec)
}
