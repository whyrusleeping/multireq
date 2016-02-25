package main

import (
	"fmt"
	"io"
	"io/ioutil"
	"log"
	"net/http"
	"net/url"
	"os"
	"strings"
	"sync"

	prometheus "github.com/prometheus/client_golang/prometheus"
)

var bufSize = 1024 * 1024 * 16

var buffers = sync.Pool{
	New: func() interface{} {
		return make([]byte, bufSize)
	},
}

func getBuffer() []byte {
	return buffers.Get().([]byte)
}

func freeBuffer(b []byte) {
	buffers.Put(b[:bufSize])
}

type buffResponse struct {
	buffered []byte
	resp     *http.Response
}

func forwardRequest(r *http.Request, target *url.URL, out chan *buffResponse, cancel chan struct{}, fail chan struct{}) {
	req_a := *r
	req_a.URL, _ = url.Parse(r.URL.String())
	req_a.URL.Host = target.Host
	req_a.Cancel = cancel

	req_a.Close = true
	rt := &http.Transport{DisableKeepAlives: true}
	resp, err := rt.RoundTrip(&req_a)
	if err != nil {
		log.Printf("target %s failed: %s", target, err)
		close(fail)
		return
	}

	if resp.StatusCode >= 500 || resp.StatusCode == 408 {
		log.Printf("target %s unsatisfying status: %d", target, resp.StatusCode)
		close(fail)

		// read entire body and close
		io.Copy(ioutil.Discard, resp.Body)
		resp.Body.Close()
		return
	}

	buf := getBuffer()

	n, err := io.ReadFull(resp.Body, buf)
	if err != nil && err != io.ErrUnexpectedEOF {
		log.Printf("%s: read error: %s", target, err)
		freeBuffer(buf)
		close(fail)
		return
	}

	out <- &buffResponse{
		buffered: buf[:n],
		resp:     resp,
	}
}

type MultiReq struct {
	TargetA *url.URL
	TargetB *url.URL
}

func (mr *MultiReq) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	resp_a := make(chan *buffResponse, 1)
	resp_b := make(chan *buffResponse, 1)

	fail_a := make(chan struct{})
	fail_b := make(chan struct{})
	fail := make(chan struct{})

	cancel_a := make(chan struct{})
	cancel_b := make(chan struct{})

	r.RequestURI = ""
	r.URL.Scheme = "http"

	go forwardRequest(r, mr.TargetA, resp_a, cancel_a, fail_a)
	go forwardRequest(r, mr.TargetB, resp_b, cancel_b, fail_b)

	go func() {
		<-fail_a
		<-fail_b
		close(fail)
	}()

	var resp *buffResponse
	select {
	case resp = <-resp_a:
		log.Printf("got %d response from a, close cancel_b", resp.resp.StatusCode)
		close(cancel_b)
	case resp = <-resp_b:
		log.Printf("got %d response from b, close cancel_a", resp.resp.StatusCode)
		close(cancel_a)
	case <-fail:
		log.Print("both failed")
		w.WriteHeader(503)
		return
	}

	for k, v := range resp.resp.Header {
		w.Header()[k] = v
	}
	w.WriteHeader(resp.resp.StatusCode)

	defer resp.resp.Body.Close()
	defer freeBuffer(resp.buffered)

	_, err := w.Write(resp.buffered)
	if err != nil {
		log.Printf("response.Write error: %s", err)
		return
	}

	written, err := io.Copy(w, resp.resp.Body)
	if err != nil {
		log.Printf("io.Copy error: %s", err)
	}
	log.Printf("io.Copy %d bytes written", written)
}

func listenAndServe(name string, addr string, h http.Handler) error {
	log.Printf("%s listening on %s", name, addr)
	err := http.ListenAndServe(addr, h)
	if err != nil {
		fmt.Println(err)
		os.Exit(1)
	}
	return nil
}

func main() {
	if len(os.Args) != 5 {
		fmt.Println("usage: multireq <listen addr> <metrics addr> <target A> <target B>")
		os.Exit(1)
	}

	listen := os.Args[1]
	listenMetrics := os.Args[2]
	targets := os.Args[3:]
	if !strings.HasPrefix(targets[0], "http") {
		fmt.Println("must specify http targets")
		os.Exit(1)
	}
	if !strings.HasPrefix(targets[1], "http") {
		fmt.Println("must specify http targets")
		os.Exit(1)
	}

	ua, err := url.Parse(targets[0])
	if err != nil {
		panic(err)
	}
	ub, err := url.Parse(targets[1])
	if err != nil {
		panic(err)
	}

	mr := &MultiReq{
		TargetA: ua,
		TargetB: ub,
	}

	go listenAndServe("metrics", listenMetrics, prometheus.Handler())
	listenAndServe("multireq", listen, prometheus.InstrumentHandler("multireq", mr))
}
