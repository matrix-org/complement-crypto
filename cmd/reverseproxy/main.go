package main

import (
	"bufio"
	"bytes"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/http/httputil"
	"net/url"
	"os"
	"strconv"
	"strings"
)

type complementReverseProxy struct {
	rp            *httputil.ReverseProxy
	controllerURL string
	client        *http.Client
}

func (p *complementReverseProxy) ServeHTTP(w http.ResponseWriter, req *http.Request) {
	dump, err := httputil.DumpRequest(req, true)
	if err != nil {
		log.Printf("DumpRequest: %s", err)
		http.Error(w, fmt.Sprint(err), http.StatusInternalServerError)
		return
	}
	// pass this to the controller to modify
	nextDump, err := p.performControllerRequest("/request", dump)
	if err != nil {
		log.Printf("performControllerRequest: %s", err)
		http.Error(w, fmt.Sprint(err), http.StatusInternalServerError)
		return
	}
	// the response is a request to send to the hs
	newReq, err := http.ReadRequest(bufio.NewReader(bytes.NewBuffer(nextDump)))
	if err != nil {
		http.Error(w, fmt.Sprint(err), http.StatusInternalServerError)
		return
	}
	// forward this new, potentially modified request, to the hs. This will call RoundTrip below.
	p.rp.ServeHTTP(w, newReq)
}

func (p *complementReverseProxy) RoundTrip(req *http.Request) (*http.Response, error) {
	// do the round trip normally
	res, err := http.DefaultTransport.RoundTrip(req)
	if err != nil {
		return res, err
	}
	// now pass the response to the controller to modify
	dump, err := httputil.DumpResponse(res, true)
	if err != nil {
		return nil, fmt.Errorf("DumpResponse: %s", err)
	}
	// the response is a potentially modified response to send to the hs
	nextDump, err := p.performControllerRequest("/response", dump)
	if err != nil {
		return nil, fmt.Errorf("performControllerRequest: %s", err)
	}
	return http.ReadResponse(bufio.NewReader(bytes.NewBuffer(nextDump)), req)
}

func (p *complementReverseProxy) performControllerRequest(path string, dump []byte) (next []byte, err error) {
	controllerReq, err := http.NewRequest("POST", p.controllerURL+path, bytes.NewBuffer(dump))
	if err != nil {
		return nil, fmt.Errorf("NewRequest: %s", err)
	}
	res, err := p.client.Do(controllerReq)
	if err != nil {
		return nil, fmt.Errorf("Do: %s", err)
	}
	if res.StatusCode != 200 {
		return nil, fmt.Errorf("Controller returned HTTP %d", res.StatusCode)
	}
	resBody, err := io.ReadAll(res.Body)
	if err != nil {
		return nil, fmt.Errorf("ReadAll: %s", err)
	}
	return resBody, nil
}

func newComplementProxy(controllerURL, urlWithPort string) (*complementReverseProxy, int) {
	segments := strings.Split(urlWithPort, ",")
	u := segments[0]
	port, err := strconv.Atoi(segments[1])
	if err != nil {
		log.Fatalf("invalid host with port: %s", urlWithPort)
	}
	if u == "" {
		log.Fatalf("invalid url: %s", urlWithPort)
	}
	uu, err := url.Parse(u)
	if err != nil {
		log.Fatalf("invalid url: %s", err)
	}
	crp := &complementReverseProxy{
		controllerURL: controllerURL,
		rp:            httputil.NewSingleHostReverseProxy(uu),
		client:        &http.Client{},
	}
	crp.rp.Transport = crp
	log.Printf("newComplementProxy on port %d : forwarding to %s", port, u)
	return crp, port
}

func main() {
	controllerURL := os.Getenv("REVERSE_PROXY_CONTROLLER_URL")
	if controllerURL == "" {
		log.Fatal("REVERSE_PROXY_CONTROLLER_URL must be set")
	}
	internalHostNames := strings.Split(os.Getenv("REVERSE_PROXY_HOSTS"), ";")
	if len(internalHostNames) == 0 || internalHostNames[0] == "" {
		log.Fatal("REVERSE_PROXY_HOSTS must be set, format = $http_url,$reverse_proxy_port; e.g REVERSE_PROXY_HOSTS=http://hs1,2000;http://hs2,2001")
	}
	for _, hostNameWithPort := range internalHostNames {
		crp, port := newComplementProxy(controllerURL, hostNameWithPort)
		go http.ListenAndServe(fmt.Sprintf(":%d", port), crp)
	}
	log.Printf("listening")
	select {} // block forever
}
