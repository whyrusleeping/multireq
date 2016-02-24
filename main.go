package main

import (
	"fmt"
	"io"
	"log"
	"net/http"
	"net/url"
	"os"
	"strings"
)

func forwardRequest(r *http.Request, target *url.URL, out chan *http.Response, cancel chan struct{}, fail chan struct{}) {
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

	if ra.StatusCode >= 500 || ra.StatusCode == 408 {
		log.Printf("target %s unsatisfying status: %d", target, ra.StatusCode)
		close(fail)
		return
	}

	out <- resp
}

func main() {
	if len(os.Args) != 4 {
		fmt.Println("usage: multireq <listen addr> <target A> <target B>")
		os.Exit(1)
	}

	listen := os.Args[1]
	targets := os.Args[2:]
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

	http.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		resp_a := make(chan *http.Response)
		resp_b := make(chan *http.Response)

		fail_a := make(chan struct{})
		fail_b := make(chan struct{})
		fail := make(chan struct{})

		cancel_a := make(chan struct{})
		cancel_b := make(chan struct{})

		r.RequestURI = ""
		r.URL.Scheme = "http"

		go forwardRequest(r, ua, resp_a, cancel_a, fail_a)
		go forwardRequest(r, ub, resp_b, cancel_b, fail_b)

		go func() {
			<-fail_a
			<-fail_b
			close(fail)
		}()

		var resp *http.Response
		select {
		case resp = <-resp_a:
			log.Print("got response from a, close cancel_b")
			close(cancel_b)
		case resp = <-resp_b:
			log.Print("got response from b, close cancel_a")
			close(cancel_a)
		case <-fail:
			log.Print("both failed")
			w.WriteHeader(503)
			return
		}

		for k, v := range resp.Header {
			w.Header()[k] = v
		}
		w.WriteHeader(resp.StatusCode)
		written, err := io.Copy(w, resp.Body)
		if err != nil {
			log.Printf("io.Copy error: %s", err)
		}
		log.Printf("io.Copy %d bytes written", written)
	})

	log.Printf("listening on %s", listen)
	err = http.ListenAndServe(listen, nil)
	if err != nil {
		fmt.Println(err)
		os.Exit(1)
	}
}
