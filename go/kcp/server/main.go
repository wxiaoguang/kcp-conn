package main

import (
	"io"
	"log"
	"math/rand"
	"net"
	"os"
	"time"
	"github.com/urfave/cli"
	"github.com/wxiaoguang/kcp-conn/go/kcp"
)

import (
	_ "net/http/pprof"
	"net/http"
	"github.com/xtaci/smux"
)

var (
	// VERSION is injected by buildflags
	VERSION = "SELFBUILD"
)

//func closeConnection(conn *net.TCPConn)  {
//	if conn != nil {
//		conn.Close()
//	}
//}

// handle multiplex-ed connection
func handleServer(kcpConn *kcp.KCPConn, config *Config) {
	// stream multiplex
	smuxConfig := smux.DefaultConfig()
	smuxConfig.MaxReceiveBuffer = config.SockBuf
	smuxConfig.KeepAliveTimeout = time.Duration(config.KeepAlive) * time.Second
	smuxConfig.KeepAliveInterval = time.Duration(config.KeepAlive) * time.Second

	mux, err := smux.Server(kcpConn, smuxConfig)
	if err != nil {
		log.Println(err)
		return
	}
	defer mux.Close()
	defer kcpConn.Close()
	for {
		p1, err := mux.AcceptStream()
		if err != nil {
			log.Println(err)
			return
		}
		p2, err := net.DialTimeout("tcp", config.Target, 5*time.Second)
		if err != nil {
			p1.Close()
			log.Println(err)
			continue
		}
		go handleClient(p1, p2, kcpConn)
	}
}

func handleClient(p1, p2 io.ReadWriteCloser, kcpCon *kcp.KCPConn) {
	log.Println("stream opened")
	defer log.Println("stream closed")
	defer p1.Close()
	defer p2.Close()

	// start tunnel
	p1die := make(chan struct{})
	go func() {
		io.Copy(p1, p2)
		close(p1die)
	}()

	p2die := make(chan struct{})
	go func() {
		io.Copy(p2, p1)
		close(p2die)
	}()

	// wait for tunnel termination
	select {
	case <-p1die:
	case <-p2die:
	}
	kcpCon.Dump()
}

//func handleServer(conn *kcp.KCPConn, tcpConn *net.TCPConn) {
//	log.Printf("stream opened: udp: local %s remote %s conv %d, tcp: local %s remote %s",
//		conn.LocalAddr(), conn.RemoteAddr(), conn.GetConv(), tcpConn.LocalAddr(), tcpConn.RemoteAddr())
//	defer log.Printf("stream closed: udp: local %s remote %s conv %d, tcp: local %s remote %s",
//		conn.LocalAddr(), conn.RemoteAddr(), conn.GetConv(), tcpConn.LocalAddr(), tcpConn.RemoteAddr())
//
//	// start tunnel
//	p1die := make(chan struct{})
//	go func() {
//		l, err0 := io.Copy(tcpConn, conn)
//		log.Printf("transfer to tcp: local %s remote %s udp: local %s remote %s conv %d failed err: %+v length: %d",
//			tcpConn.LocalAddr(), tcpConn.RemoteAddr(), conn.LocalAddr(), conn.RemoteAddr(), conn.GetConv(), err0, l)
//		close(p1die)
//	}()
//
//	p2die := make(chan struct{})
//	go func() {
//		l, err1 := io.Copy(conn, tcpConn)
//		log.Printf("transfer to udp: local %s remote %s conv %d tcp: local %s remote %s failed err: %+v length: %d",
//			conn.LocalAddr(), conn.RemoteAddr(), conn.GetConv(), tcpConn.LocalAddr(), tcpConn.RemoteAddr(), err1, l)
//		close(p2die)
//	}()
//
//	// wait for tunnel termination
//	select {
//	case <-p1die:
//	case <-p2die:
//	}
//
//	log.Printf("hanelServer call Close udp local %s conv %d because io failed", conn.LocalAddr(), conn.GetConv())
//	conn.Dump()
//	conn.Close()
//	closeConnection(tcpConn)
//}

func checkError(err error) {
	if err != nil {
		log.Printf("%+v\n", err)
		os.Exit(-1)
	}
}

func main() {
	rand.Seed(int64(time.Now().Nanosecond()))
	if VERSION == "SELFBUILD" {
		// add more log flags for debugging
		log.SetFlags(log.LstdFlags |log.Lmicroseconds | log.Lshortfile)
	}
	myApp := cli.NewApp()
	myApp.Name = "kcpconn"
	myApp.Usage = "server"
	myApp.Version = VERSION
	myApp.Flags = []cli.Flag{
		cli.StringFlag{
			Name:  "listen,l",
			Value: ":29900",
			Usage: "kcp server listen address",
		},
		cli.StringFlag{
			Name:  "target, t",
			Value: "127.0.0.1:12948",
			Usage: "target server address",
		},
		cli.StringFlag{
			Name:  "mode",
			Value: "fast",
			Usage: "profiles: fast4, fast3, fast2, fast, normal",
		},
		cli.IntFlag{
			Name:  "mtu",
			Value: 1350,
			Usage: "set maximum transmission unit for UDP packets",
		},
		cli.IntFlag{
			Name:  "sndwnd",
			Value: 1024,
			Usage: "set send window size(num of packets)",
		},
		cli.IntFlag{
			Name:  "rcvwnd",
			Value: 1024,
			Usage: "set receive window size(num of packets)",
		},
		cli.IntFlag{
			Name:   "nodelay",
			Value:  0,
			Hidden: true,
		},
		cli.IntFlag{
			Name:   "interval",
			Value:  40,
			Hidden: true,
		},
		cli.IntFlag{
			Name:   "resend",
			Value:  0,
			Hidden: true,
		},
		cli.IntFlag{
			Name:   "nc",
			Value:  0,
			Hidden: true,
		},
		cli.IntFlag{
			Name:   "sockbuf",
			Value:  4194304, // socket buffer size in bytes
			Hidden: true,
		},
		cli.IntFlag{
			Name:   "keepalive",
			Value:  10, // nat keepalive interval in seconds
			Hidden: true,
		},
		cli.StringFlag{
			Name:  "log",
			Value: "",
			Usage: "specify a log file to output, default goes to stderr",
		},
		cli.StringFlag{
			Name:  "pprof",
			Value: ":6060",
			Usage: "pprof addr, default is :6060",
		},
		cli.StringFlag{
			Name:  "c",
			Value: "", // when the value is not empty, the config path must exists
			Usage: "config from json file, which will override the command from shell",
		},
	}
	myApp.Action = func(c *cli.Context) error {
		config := Config{}
		config.Listen = c.String("listen")
		config.Target = c.String("target")
		config.Mode = c.String("mode")
		config.MTU = c.Int("mtu")
		config.SndWnd = c.Int("sndwnd")
		config.RcvWnd = c.Int("rcvwnd")
		config.NoDelay = c.Int("nodelay")
		config.Interval = c.Int("interval")
		config.Resend = c.Int("resend")
		config.NoCongestion = c.Int("nc")
		config.SockBuf = c.Int("sockbuf")
		config.KeepAlive = c.Int("keepalive")
		config.Log = c.String("log")
		config.PPROF = c.String("pprof")

		if c.String("c") != "" {
			//Now only support json config file
			err := parseJSONConfig(&config, c.String("c"))
			checkError(err)
		}

		// log redirect
		if config.Log != "" {
			f, err := os.OpenFile(config.Log, os.O_RDWR|os.O_CREATE|os.O_APPEND, 0666)
			checkError(err)
			defer f.Close()
			os.Stdout = f
			os.Stderr = f
			log.SetOutput(f)
		}

		switch config.Mode {
		case "normal":
			config.NoDelay, config.Interval, config.Resend, config.NoCongestion = 0, 30, 2, 1
		case "fast":
			config.NoDelay, config.Interval, config.Resend, config.NoCongestion = 0, 20, 2, 1
		case "fast2":
			config.NoDelay, config.Interval, config.Resend, config.NoCongestion = 1, 20, 2, 1
		case "fast3":
			config.NoDelay, config.Interval, config.Resend, config.NoCongestion = 1, 10, 2, 1
		case "fast4":
			config.NoDelay, config.Interval, config.Resend, config.NoCongestion = 1, 10, 1, 1
		}

		log.Println("version:", VERSION)

		lis, err := kcp.Listen(config.Listen)
		checkError(err)

		log.Println("listening on:", lis.Addr())
		log.Println("target:", config.Target)
		log.Println("nodelay parameters:", config.NoDelay, config.Interval, config.Resend, config.NoCongestion)
		log.Println("sndwnd:", config.SndWnd, "rcvwnd:", config.RcvWnd)
		log.Println("mtu:", config.MTU)
		log.Println("sockbuf:", config.SockBuf)
		log.Println("keepalive:", config.KeepAlive)
		log.Println("version:", VERSION)
		log.Println("pprof:", config.PPROF)

		go func() {
			log.Println(http.ListenAndServe(config.PPROF, nil))
		}()

		for {
			if conn, err := lis.Accept(); err == nil {
				log.Printf("accept new udp connection: %s", conn.RemoteAddr())
				kcpConn := conn.(*kcp.KCPConn)
				kcpConn.SetStreamMode(true)
				kcpConn.SetNoDelay(config.NoDelay, config.Interval, config.Resend, config.NoCongestion)
				kcpConn.SetMtu(config.MTU)
				kcpConn.SetWindowSize(config.SndWnd, config.RcvWnd)
				kcpConn.SetKeepAlive(config.KeepAlive)

				go handleServer(kcpConn, &config)

				//conn, err := net.DialTimeout("tcp", config.Target, 60*time.Second)
				//if err != nil {
				//	log.Printf("create new tcp: %s connection error: %+v", config.Target, err)
				//	continue
				//}
				//tcpConn := conn.(*net.TCPConn)
				//if err := tcpConn.SetReadBuffer(config.SockBuf); err != nil {
				//	log.Printf("TCP local %s remote %s SetReadBuffer error: %+v",
				//		tcpConn.LocalAddr(), tcpConn.RemoteAddr(), err)
				//}
				//if err := tcpConn.SetWriteBuffer(config.SockBuf); err != nil {
				//	log.Printf("TCP local %s remote %s SetWriteBuffer error: %+v",
				//		tcpConn.LocalAddr(), tcpConn.RemoteAddr(), err)
				//}
				//go handleServer(kcpConn, tcpConn)
			} else {
				log.Printf("accept new udp connection error: %+v", err)
			}
		}

	}
	myApp.Run(os.Args)
}
