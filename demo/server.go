package main

import (
	"log"
	"net/http"
	"crypto/tls"
	"github.com/wxiaoguang/kcp-conn/go/kcp"
)

type DemoHttpServerHandler struct { }

func (s *DemoHttpServerHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	w.Write([]byte("Hello, this is a http server"))
}

func ServeKcp(addr string, tlsConfig *tls.Config) {
	handler := &DemoHttpServerHandler{}
	server := &http.Server{Handler: handler, TLSConfig: tlsConfig}
	ln, err := kcp.Listen(addr)
	if err != nil {
		log.Panic("failed to listen", err)
		return
	}

	if tlsConfig != nil {
		log.Println("listening https on " + addr)
		ln = tls.NewListener(ln, tlsConfig)
	} else {
		log.Println("listening http on " + addr)
	}

	go server.Serve(ln)
}

func main() {

	// kcp http
	ServeKcp("127.0.0.1:8880", nil)

	// kcp https
	var err error
	tlsConfig := &tls.Config{}
	tlsConfig.Certificates = make([]tls.Certificate, 1)
	tlsConfig.Certificates[0], err = tls.LoadX509KeyPair("tls-demo.crt", "tls-demo.key")
	if err != nil {
		log.Panic("cert error: ", err)
		return
	}
	tlsConfig.BuildNameToCertificate()
	ServeKcp("127.0.0.1:8883", tlsConfig)

	//wait forever
	<- make(chan int)
}
