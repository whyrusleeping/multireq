package main

import (
	"fmt"
	"io/ioutil"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"
	"time"
)

type response struct {
	Code    int
	Content string
	Delay   time.Duration
}

func (rr *response) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if rr.Delay != 0 {
		time.Sleep(rr.Delay)
	}

	w.WriteHeader(rr.Code)
	w.Write([]byte(rr.Content))
}

func (rr *response) matches(hr *http.Response) error {
	if hr.StatusCode != rr.Code {
		return fmt.Errorf("got code of %d when we expected %d", hr.StatusCode, rr.Code)
	}

	data, err := ioutil.ReadAll(hr.Body)
	if err != nil {
		return err
	}

	if string(data) != rr.Content {
		return fmt.Errorf("got data of %q, but expected %q", data, rr.Content)
	}

	return nil
}

type mockServer struct {
	RespA *response
	RespB *response

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

func subtestResponses(t *testing.T, a, b *response, exp *response) {
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
