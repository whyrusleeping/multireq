package main

import (
	"fmt"
	"io"
	"io/ioutil"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"
	"time"
)

type response struct {
	Code    int
	Content interface{}
	Delay   time.Duration
}

func (rr *response) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if rr.Delay != 0 {
		time.Sleep(rr.Delay)
	}

	w.WriteHeader(rr.Code)
	switch c := rr.Content.(type) {
	case string:
		w.Write([]byte(c))
	case []byte:
		w.Write(c)
	case io.Reader:
		io.Copy(w, c)
	}
}

func (rr *response) matches(hr *http.Response) error {
	if hr.StatusCode != rr.Code {
		return fmt.Errorf("got code of %d when we expected %d", hr.StatusCode, rr.Code)
	}

	data, err := ioutil.ReadAll(hr.Body)
	if err != nil {
		return err
	}

	switch exp := rr.Content.(type) {
	case string:
		if string(data) != exp {
			return fmt.Errorf("got data of %q, but expected %q", data, rr.Content)
		}
	default:
		return fmt.Errorf("cant handle this yet")
	}

	return nil
}

type mockServer struct {
	RespA http.Handler
	RespB http.Handler

	ServerA *httptest.Server
	ServerB *httptest.Server
}

func (ms *mockServer) Serve(t *testing.T) {
	ms.ServerA = httptest.NewServer(ms.RespA)
	ms.ServerB = httptest.NewServer(ms.RespB)
}

func (ms *mockServer) Close() {
	ms.ServerA.Close()
	ms.ServerB.Close()
}

func (ms *mockServer) makeMultireq() *MultiReq {
	ua, _ := url.Parse(ms.ServerA.URL)
	ub, _ := url.Parse(ms.ServerB.URL)

	return &MultiReq{
		TargetA: ua,
		TargetB: ub,
	}
}

func subtestResponses(t *testing.T, a, b http.Handler, exp *response) {
	ms := &mockServer{
		RespA: a,
		RespB: b,
	}
	ms.Serve(t)

	server := httptest.NewServer(ms.makeMultireq())

	resp, err := http.Get(server.URL)
	if err != nil {
		t.Fatal(err)
	}

	err = exp.matches(resp)
	if err != nil {
		t.Fatal(err)
	}

}

func TestFastestWins(t *testing.T) {
	a := &response{
		Code:    200,
		Content: "this is good",
	}
	b := &response{
		Code:    200,
		Content: "also good, just slow",
		Delay:   time.Second,
	}

	subtestResponses(t, a, b, a)
}

func Test404Okay(t *testing.T) {
	a := &response{
		Code:    404,
		Content: "blah blah not found",
	}
	b := &response{
		Code:    404,
		Content: "not found still",
		Delay:   time.Second,
	}

	subtestResponses(t, a, b, a)
}

func TestReturnsSlowerGoodResponse(t *testing.T) {
	a := &response{
		Code:    503,
		Content: "teh serverman ded",
	}
	b := &response{
		Code:    200,
		Content: "hey look! content!",
		Delay:   time.Millisecond * 100,
	}

	subtestResponses(t, a, b, b)
}

func TestBuffering(t *testing.T) {
	normbufsize := bufSize
	bufSize = 16
	defer func() {
		bufSize = normbufsize
	}()

	r, w := io.Pipe()
	a := &response{
		Code:    200,
		Content: r,
	}
	b := &response{
		Code:    200,
		Content: "hey look! content that is longer than sixteen characters!",
		Delay:   time.Millisecond * 100,
	}

	go func() {
		w.Write([]byte("a few bytes"))
		// now hang
		time.Sleep(time.Second)

		w.Close()
	}()

	subtestResponses(t, a, b, b)

}
