package kcp

import (
    "fmt"
    "log"
    "net"
    "sync"
    "testing"
    "time"
    "io"
)

var testServerAddr net.Addr
var testServerListener *Listener

func init() {
    ln, err := Listen(":")
    testServerListener = ln.(*Listener)
    testServerListener.SetReadBuffer(16 * 1024 * 1024)
    testServerListener.SetWriteBuffer(16 * 1024 * 1024)
    if err != nil {
        panic(err)
    }
    testServerAddr = testServerListener.Addr()
    log.Println("listening on:", testServerAddr.String())

    go server()
}

func server() {
    for {
        c, err := testServerListener.Accept()
        if err != nil {
            panic(err)
        }
        kcpConn := c.(*KCPConn)
        kcpConn.SetNoDelay(1, 20, 2, 1)
        kcpConn.SetWindowSize(1024, 1024) // faster
        kcpConn.SetReadBuffer(16 * 1024 * 1024)
        kcpConn.SetWriteBuffer(16 * 1024 * 1024)
        kcpConn.SetKeepAlive(1)
        go handleClient(c)
    }
}

func DialTest() (*KCPConn, error) {
    return DialTimeout("udp", testServerAddr.String(), time.Second)
}

// FIXME: all uncovered codes
func TestCoverage(t *testing.T) {
    DialTimeout("udp", testServerAddr.String(), time.Second)
}

func handleClient(conn net.Conn) {
    conn.SetReadDeadline(time.Now().Add(time.Hour))
    conn.SetWriteDeadline(time.Now().Add(time.Hour))
    //fmt.Println("new client", conn.RemoteAddr())
    buf := make([]byte, 65536)
    count := 0
    recvSize := 0
    for {
        n, err := conn.Read(buf)
        if err != nil && err != io.EOF {
            panic(err)
        }
        if n == 0 {
            log.Printf("server: %+v\n", conn.(*KCPConn).kcp.stats)
            break
        }
        count++
        recvSize += n
        _, err = conn.Write(buf[:n])
        if err != nil {
            panic(err)
        }
    }
    conn.Close()
}

func TestTimeout(t *testing.T) {
    cli, err := DialTest()
    if err != nil {
        panic(err)
    }
    buf := make([]byte, 10)

    //timeout
    cli.SetDeadline(time.Now().Add(time.Second))
    <-time.After(2 * time.Second)
    n, err := cli.Read(buf)
    if n != 0 || err == nil {
        t.Fail()
    }
}

func TestClose(t *testing.T) {
    cli, err := DialTest()
    if err != nil {
        panic(err)
    }
    buf := make([]byte, 10)

    cli.Close()
    if cli.Close() == nil {
        t.Fail()
    }
    n, err := cli.Write(buf)
    if n != 0 || err == nil {
        t.Fail()
    }
    n, err = cli.Read(buf)
    if n != 0 || err == nil {
        t.Fail()
    }
}

func TestSendRecv(t *testing.T) {
    var wg sync.WaitGroup
    const par = 1
    wg.Add(par)
    for i := 0; i < par; i++ {
        go testSendRecvClient(&wg)
    }
    wg.Wait()
}

func testSendRecvClient(wg *sync.WaitGroup) {
    cli, err := DialTest()
    if err != nil {
        panic(err)
    }
    cli.SetReadBuffer(16 * 1024 * 1024)
    cli.SetWriteBuffer(16 * 1024 * 1024)
    cli.SetNoDelay(1, 20, 2, 1)
    //cli.SetACKNoDelay(true)
    cli.SetDeadline(time.Now().Add(time.Minute))
    const N = 1000
    buf := make([]byte, 10)
    for i := 0; i < N; i++ {
        msg := fmt.Sprintf("hello%v", i)
        fmt.Println("sent:", msg)
        cli.Write([]byte(msg))
        if n, err := cli.Read(buf); err == nil {
            fmt.Println("recv:", string(buf[:n]))
        } else {
            panic(err)
        }
    }
    cli.Close()
    wg.Done()
}

func TestSpeed(t *testing.T) {
    var wg sync.WaitGroup
    wg.Add(1)
    go testSpeedClient(&wg, 1, 4096, 4096 * 10)
    wg.Wait()
}

func testSpeedClient(wg *sync.WaitGroup, nc, msgSize, count int) {
    cli, err := DialTest()
    if err != nil {
        panic(err)
    }
    log.Println("remote:", cli.RemoteAddr(), "local:", cli.LocalAddr(), "conv:", cli.GetConv(), "expected size:", msgSize * count)
    cli.SetWindowSize(1024, 1024)
    cli.SetNoDelay(1, 20, 2, nc)
    start := time.Now()

    go func() {
        buf := make([]byte, 1024*1024)
        nrecv := 0
        for {
            n, err := cli.Read(buf)
            if err != nil {
                log.Println(err)
                break
            } else {
                nrecv += n
                //log.Printf("nrecv = %d\n", nrecv)
                if nrecv == msgSize*count {
                    break
                }
            }
        }

        println("total recv:", nrecv)
        cli.Close()
        mb := (float64(msgSize)*float64(count)/1024.0/1024.0)
        d := time.Now().Sub(start)

        <- time.After(500 * time.Millisecond)

        log.Printf("time for %.2f MB : %v, speed=%.2f MB/s\n", mb, d, mb / d.Seconds())
        log.Printf("client: %+v\n", cli.kcp.stats)
        log.Printf("stats: %+v\n", Stats)
        wg.Done()
    }()
    msg := make([]byte, msgSize)
    cli.SetWindowSize(1024, 1024)
    for i := 0; i < count; i++ {
        cli.Write(msg)
    }
}

func TestParallel(t *testing.T) {
    par := 200
    var wg sync.WaitGroup
    wg.Add(par)
    log.Println("testing parallel", par, "connections")
    for i := 0; i < par; i++ {
        go testParallelClient(i, &wg)
    }
    wg.Wait()
    log.Println("testing parallel", par, "connections done")
}

func testParallelClient(idx int, wg *sync.WaitGroup) {
    cli, err := DialTest()
    if err != nil {
        panic(err)
    }
    const N = 100
    cli.SetNoDelay(1, 20, 2, 1)
    buf := make([]byte, 10)
    for i := 0; i < N; i++ {
        cli.SetDeadline(time.Now().Add(3 * time.Second))
        msg := fmt.Sprintf("hello%v", i)
        cli.Write([]byte(msg))
        if _, err := cli.Read(buf); err != nil {
            log.Printf("%d %d error\n", idx, i)
            break
        }
        //log.Printf("%d %d\n", idx, i)
        <-time.After(10 * time.Millisecond)
    }
    cli.Close()
    wg.Done()
}
