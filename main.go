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
		fmt.Println("usage: multireq <listen addr> <target 1> <target 2>")
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
		a := make(chan *http.Response)
		b := make(chan *http.Response)

		r.RequestURI = ""
		r.URL.Scheme = "http"
		req_a := *r
		req_b := *r
		req_a.URL, _ = url.Parse(r.URL.String())
		req_a.URL.Host = ua.Host
		req_b.URL, _ = url.Parse(r.URL.String())
		req_b.URL.Host = ub.Host

		cancel_a := make(chan struct{})
		cancel_b := make(chan struct{})

		req_a.Cancel = cancel_a
		req_b.Cancel = cancel_b

		go func() {
			rt := &http.Transport{DisableKeepAlives: true}
			resp, err := rt.RoundTrip(&req_a)
			if err != nil {
				log.Printf("target 1 failed: %s", err)
				return
			}

			a <- resp
		}()

		go func() {
			rt := &http.Transport{DisableKeepAlives: true}
			resp, err := rt.RoundTrip(&req_a)
			if err != nil {
				log.Printf("target 2 failed: %s", err)
				return
			}

			b <- resp
		}()

		failed := make(chan struct{})
		go func() {
			<-cancel_a
			<-cancel_b
			close(failed)
		}()

		var resp *http.Response
		select {
		case resp = <-a:
			if resp.StatusCode < 400 {
				close(cancel_b)
			}
		case resp = <-b:
			if resp.StatusCode < 400 {
				close(cancel_a)
			}
		case <-failed:
			w.WriteHeader(404)
			return
		}

		for k, v := range resp.Header {
			w.Header()[k] = v
		}
		w.WriteHeader(resp.StatusCode)
		io.Copy(w, resp.Body)
	})

	log.Printf("listening on %s", listen)
	err = http.ListenAndServe(listen, nil)
	if err != nil {
		fmt.Println(err)
		os.Exit(1)
	}
}
