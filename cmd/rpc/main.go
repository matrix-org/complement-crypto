package main

import (
	"fmt"
	"log"
	"net"
	"net/http"
	"net/rpc"

	crpc "github.com/matrix-org/complement-crypto/internal/deploy/rpc"
)

func main() {
	srv := crpc.NewServer()
	rpc.Register(srv)
	rpc.HandleHTTP()
	listener, err := net.Listen("tcp", ":0")
	if err != nil {
		log.Fatal("Listener error: ", err)
	}
	// tell the parent process what port we are listening on.
	port := listener.Addr().(*net.TCPAddr).Port
	fmt.Println(port)
	fmt.Println(http.Serve(listener, nil))
}
