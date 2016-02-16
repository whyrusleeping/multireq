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

		go func() {
			req_a := *r
			req_a.URL.Host = ua.Host
			req_a.URL, _ = url.Parse(r.URL.String())
			req_a.Cancel = cancel_a

			rt := &http.Transport{DisableKeepAlives: true}
			resp, err := rt.RoundTrip(&req_a)
			if err != nil {
				log.Printf("target A failed: %s", err)
				close(fail_a)
			} else if resp.StatusCode >= 500 || resp.StatusCode == 408 {
				log.Printf("target A unsatisfying status: %d", resp.StatusCode)
				close(fail_a)
			} else {
				resp_a <- resp
			}
		}()

		go func() {
			req_b := *r
			req_b.URL, _ = url.Parse(r.URL.String())
			req_b.URL.Host = ub.Host
			req_b.Cancel = cancel_b

			rt := &http.Transport{DisableKeepAlives: true}
			resp, err := rt.RoundTrip(&req_b)
			if err != nil {
				log.Printf("target B failed: %s", err)
				close(fail_b)
			} else if resp.StatusCode >= 500 || resp.StatusCode == 408 {
				log.Printf("target B unsatisfying status: %d", resp.StatusCode)
				close(fail_b)
			} else {
				resp_b <- resp
			}
		}()

		go func() {
			<-fail_a
			<-fail_b
			close(fail)
		}()

		var ra *http.Response
		var rb *http.Response
		var resp *http.Response
		done := false
	OuterLoop:
		for {
			select {
			case ra = <-resp_a:
				if !done {
					done = true
					resp = ra
					log.Print("close cancel_b")
					close(cancel_b)
					break OuterLoop
				}
			case rb = <-resp_b:
				if !done {
					done = true
					resp = rb
					log.Print("close cancel_a")
					close(cancel_a)
					break OuterLoop
				}
			case <-fail:
				log.Print("both failed")
				break OuterLoop
			}
		}

		if !done {
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
