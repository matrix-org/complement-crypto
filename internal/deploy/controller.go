package deploy

import (
	"fmt"
	"net"
	"net/http"
)

type ReverseProxyController struct {
	srv *http.Server
}

func NewReverseProxyController() *ReverseProxyController {
	return &ReverseProxyController{}
}

func (c *ReverseProxyController) ServeHTTP(w http.ResponseWriter, req *http.Request) {

}

func (c *ReverseProxyController) Listen() (port int, err error) {
	listener, err := net.Listen("tcp", ":0")
	if err != nil {
		return 0, fmt.Errorf("net.Listen failed: %s", err)
	}
	port = listener.Addr().(*net.TCPAddr).Port
	c.srv = &http.Server{Addr: ":0", Handler: c}
	go c.srv.Serve(listener)
	return port, nil
}

func (c *ReverseProxyController) Terminate() {
	c.srv.Close()
}
