package main

import (
	"context"
	"fmt"
	"log"
	"net"
	"net/http"
	"net/rpc"

	crpc "github.com/matrix-org/complement-crypto/internal/deploy/rpc"
)

func main() {
	srvDoneChannel := make(chan struct{})
	srv := crpc.NewServer(srvDoneChannel)
	rpc.Register(srv)
	rpc.HandleHTTP()
	listener, err := net.Listen("tcp", ":0")
	if err != nil {
		log.Fatal("Listener error: ", err)
	}
	// tell the parent process what port we are listening on.
	port := listener.Addr().(*net.TCPAddr).Port
	fmt.Println(port)

	// Start an HTTP server on the listener. It will send requests to the default HttpMux, which we've
	// already told rpc to handle.
	httpServer := &http.Server{}

	go httpServer.Serve(listener)

	// Wait for `Server.Close` to be called, then shut down the HTTP server.
	<- srvDoneChannel
	fmt.Println("Server starting clean shutdown...")
	err = httpServer.Shutdown(context.Background())
	if err != nil {
		log.Fatal("HTTP server Shutdown error: ", err)
	}

	fmt.Println("Server shutdown complete")
}
