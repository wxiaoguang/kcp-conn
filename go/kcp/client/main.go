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
	//"container/list"
	//"sync"
	"github.com/xtaci/smux"
)

var (
	// VERSION is injected by buildflags
	VERSION = "SELFBUILD"
)


func handleClient(sess *smux.Session, kcpConn *kcp.KCPConn, p1 io.ReadWriteCloser) {
	log.Println("stream opened")
	defer log.Println("stream closed")
	defer p1.Close()
	p2, err := sess.OpenStream()
	if err != nil {
		return
	}
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
	kcpConn.Dump()
}

//func handleClient(conn *kcp.KCPConn, tcpConn *net.TCPConn) {
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
//	log.Printf("hanelClient call Close udp local %s conv %d because io failed", conn.LocalAddr(), conn.GetConv())
//	conn.Dump()
//	conn.Close()
//	closeConnection(tcpConn)
//	createConnectionForFuture()
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
	myApp.Usage = "client"
	myApp.Version = VERSION
	myApp.Flags = []cli.Flag{
		cli.StringFlag{
			Name:  "localaddr,l",
			Value: ":12948",
			Usage: "local listen address",
		},
		cli.StringFlag{
			Name:  "remoteaddr, r",
			Value: "vps:29900",
			Usage: "kcp server address",
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
			Value: 128,
			Usage: "set send window size(num of packets)",
		},
		cli.IntFlag{
			Name:  "rcvwnd",
			Value: 512,
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
		cli.IntFlag{
			Name:  "conn",
			Value: 1,
			Usage: "set num of UDP connections to server",
		},
		cli.StringFlag{
			Name:  "c",
			Value: "", // when the value is not empty, the config path must exists
			Usage: "config from json file, which will override the command from shell",
		},
	}
	myApp.Action = func(c *cli.Context) error {
		config.LocalAddr = c.String("localaddr")
		config.RemoteAddr = c.String("remoteaddr")
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
		config.CONN = c.Int("conn")

		if c.String("c") != "" {
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

		addr, err := net.ResolveTCPAddr("tcp", config.LocalAddr)
		checkError(err)
		listener, err := net.ListenTCP("tcp", addr)
		checkError(err)

		log.Println("listening on:", listener.Addr())
		log.Println("nodelay parameters:", config.NoDelay, config.Interval, config.Resend, config.NoCongestion)
		log.Println("remote address:", config.RemoteAddr)
		log.Println("sndwnd:", config.SndWnd, "rcvwnd:", config.RcvWnd)
		log.Println("mtu:", config.MTU)
		log.Println("sockbuf:", config.SockBuf)
		log.Println("keepalive:", config.KeepAlive)
		log.Println("pprof:", config.PPROF)

		go func() {
			log.Println(http.ListenAndServe(config.PPROF, nil))
		}()

		//refreshConnectionPool()

		//go func() {
		//	for {
		//		select {
		//		case <-time.After(time.Duration(config.KeepAlive) * time.Second):
		//		}
		//		refreshConnectionPool()
		//	}
		//}()

		// wait until a connection is ready
		waitConn := func() (*smux.Session, *kcp.KCPConn) {
			log.Print("waitConn begin")
			defer log.Print("waitConn end")
			for {
				if session, kcpConn, err := createConn(); err == nil {
					return session, kcpConn
				} else {
					log.Printf("waitConn failed because %+v", err)
					time.Sleep(time.Second)
				}
			}
		}

		rr := uint16(0)
		muxes := make([]struct {
			session *smux.Session
			kcpConn *kcp.KCPConn
		}, config.CONN)
		for k := range muxes {
			muxes[k].session, muxes[k].kcpConn = waitConn()
		}

		for {
			tcpConn, err := listener.AcceptTCP()
			if err != nil {
				log.Printf("accept new tcp connection error: %+v", err)
				closeConnection(tcpConn)
				continue
			}
			log.Printf("accept new tcp connection: %s", tcpConn.RemoteAddr())
			if err := tcpConn.SetReadBuffer(config.SockBuf); err != nil {
				log.Printf("TCP local %s remote %s SetReadBuffer error: %+v",
					tcpConn.LocalAddr(), tcpConn.RemoteAddr(), err)
			}
			if err := tcpConn.SetWriteBuffer(config.SockBuf); err != nil {
				log.Printf("TCP local %s remote %s SetWriteBuffer error: %+v",
					tcpConn.LocalAddr(), tcpConn.RemoteAddr(), err)
			}

			idx := rr % uint16(config.CONN)
			rr++

			// do auto expiration && reconnection
			if muxes[idx].session.IsClosed() || muxes[idx].kcpConn.IsClosed() {
				muxes[idx].session, muxes[idx].kcpConn = waitConn()
			}

			go handleClient(muxes[idx].session, muxes[idx].kcpConn, tcpConn)
			//conn, err := getConn()
			//if err != nil {
			//	log.Printf("getConn udp: %s failed because error: %+v", config.RemoteAddr, err)
			//	closeConnection(tcpConn)
			//	continue
			//}
			//go handleClient(conn, tcpConn)
		}
	}
	myApp.Run(os.Args)
}

var (
	//connectionPool = list.New()
	config = Config{}
	//mutex = new(sync.Mutex)
)

func createConn() (*smux.Session, *kcp.KCPConn, error) {
	log.Printf("create udp: %s begin ", config.RemoteAddr)
	defer log.Printf("create udp: %s end ", config.RemoteAddr)
	kcpconn, err := kcp.DialTimeout("udp", config.RemoteAddr, 10*time.Second)
	if err != nil {
		return nil, nil, err
	}
	kcpconn.SetStreamMode(true)
	kcpconn.SetNoDelay(config.NoDelay, config.Interval, config.Resend, config.NoCongestion)
	kcpconn.SetWindowSize(config.SndWnd, config.RcvWnd)
	kcpconn.SetMtu(config.MTU)
	kcpconn.SetKeepAlive(config.KeepAlive)

	if err := kcpconn.SetReadBuffer(config.SockBuf); err != nil {
		log.Println("SetReadBuffer:", err)
	}
	if err := kcpconn.SetWriteBuffer(config.SockBuf); err != nil {
		log.Println("SetWriteBuffer:", err)
	}

	// stream multiplex
	smuxConfig := smux.DefaultConfig()
	smuxConfig.MaxReceiveBuffer = config.SockBuf
	smuxConfig.KeepAliveTimeout = time.Duration(config.KeepAlive) * time.Second
	smuxConfig.KeepAliveInterval = time.Duration(config.KeepAlive) * time.Second
	session, err1 := smux.Client(kcpconn, smuxConfig)

	if err1 != nil {
		return nil, nil, err1
	}
	return session, kcpconn, nil
}
//func createConn () (*kcp.KCPConn, error) {
//	log.Printf("create udp: %s begin ", config.RemoteAddr)
//	defer log.Printf("create udp: %s end ", config.RemoteAddr)
//	conn, err := kcp.DialTimeout("udp", config.RemoteAddr, 10 * time.Second)
//	if err != nil {
//		return nil, err
//	}
//	conn.SetStreamMode(true)
//	conn.SetNoDelay(config.NoDelay, config.Interval, config.Resend, config.NoCongestion)
//	conn.SetWindowSize(config.SndWnd, config.RcvWnd)
//	conn.SetMtu(config.MTU)
//	conn.SetKeepAlive(config.KeepAlive)
//
//	if err := conn.SetReadBuffer(config.SockBuf); err != nil {
//		log.Printf("KCP local %s remote %s conv %d SetReadBuffer error: %+v",
//			conn.LocalAddr(), conn.RemoteAddr(), conn.GetConv(), err)
//	}
//	if err := conn.SetWriteBuffer(config.SockBuf); err != nil {
//		log.Printf("KCP local %s remote %s conv %d SetWriteBuffer error: %+v",
//			conn.LocalAddr(), conn.RemoteAddr(), conn.GetConv(), err)
//	}
//
//	return conn, nil
//}

//func getConn() (*kcp.KCPConn, error) {
//
//	mutex.Lock()
//	var conn *kcp.KCPConn
//	conn = nil
//	for index := connectionPool.Front(); index != nil; {
//		indexNext := index.Next()
//		conn = index.Value.(*kcp.KCPConn)
//		connectionPool.Remove(index)
//		if conn.IsClosed() {
//			conn = nil
//		} else {
//			break
//		}
//		index = indexNext
//	}
//	mutex.Unlock()
//	if conn == nil {
//		log.Printf("getConn create udp: %s because connection pool is empty", config.RemoteAddr)
//		var err error
//		conn, err = createConn()
//		if err != nil {
//			log.Printf("getConn create udp: %s failed because error: %+v", config.RemoteAddr, err)
//			return nil, err
//		}
//		mutex.Lock()
//		connectionPool.PushBack(conn)
//		mutex.Unlock()
//	}
//	log.Printf("getConn get from connection pool udp local: %s conv: %d success", conn.LocalAddr(), conn.GetConv())
//	return conn, nil
//}
//
//func createConnectionForFuture() {
//	conn, err := createConn()
//	if err != nil {
//		log.Printf("createConnectionForFuture create udp: %s failed because error: %+v", config.RemoteAddr, err)
//		return
//	}
//	mutex.Lock()
//	connectionPool.PushBack(conn)
//	mutex.Unlock()
//	log.Printf("createConnectionForFuture create udp local: %s conv: %d success", conn.LocalAddr(), conn.GetConv())
//}
//
//func refreshConnectionPool() {
//	log.Print("refreshConnectionPool begin")
//	defer log.Print("refreshConnectionPool end")
//	mutex.Lock()
//	var connectionPoolSize int
//	var conn *kcp.KCPConn
//	for index := connectionPool.Front(); index != nil; {
//		indexNext := index.Next()
//		conn = index.Value.(*kcp.KCPConn)
//		if conn.IsClosed() {
//			connectionPool.Remove(index)
//		}
//		index = indexNext
//	}
//	connectionPoolSize = connectionPool.Len()
//	mutex.Unlock()
//
//	shouldAddSize := 10 - connectionPoolSize
//
//	newAddList := list.New()
//	for ;shouldAddSize >0; shouldAddSize-- {
//		newConn, err := createConn()
//		if err == nil {
//			newAddList.PushBack(newConn)
//		}
//	}
//
//	mutex.Lock()
//	connectionPool.PushBackList(newAddList)
//	mutex.Unlock()
//}

func closeConnection(conn *net.TCPConn)  {
	if conn != nil {
		conn.Close()
	}
}
