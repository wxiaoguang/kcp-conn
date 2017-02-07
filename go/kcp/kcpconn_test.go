package kcp

import (
    "fmt"
    "log"
    "net"
    _ "net/http"
    _ "net/http/pprof"
    "sync"
    "testing"
    "time"
)

const port = "127.0.0.1:9999"

func init() {
    /*
    go func() {
        log.Println(http.ListenAndServe("localhost:6060", nil))
    }()
    */
    go server()
    <-time.After(200 * time.Millisecond)
}

func DialTest() (*KCPConn, error) {
    return DialTimeout("udp", port, time.Second)
}

// all uncovered codes
func TestCoverage(t *testing.T) {
    DialTimeout("udp", "127.0.0.1:100000", time.Second)
}

func ListenTest() (net.Listener, error) {
    return Listen(port)
}

func server() {
    l, err := ListenTest()
    if err != nil {
        panic(err)
    }

    kcplistener := l.(*Listener)
    kcplistener.SetReadBuffer(16 * 1024 * 1024)
    kcplistener.SetWriteBuffer(16 * 1024 * 1024)
    log.Println("listening on:", kcplistener.conn.LocalAddr())
    for {
        s, err := l.Accept()
        if err != nil {
            panic(err)
        }

        // coverage test
        s.(*KCPConn).SetReadBuffer(16 * 1024 * 1024)
        s.(*KCPConn).SetWriteBuffer(16 * 1024 * 1024)
        s.(*KCPConn).SetKeepAlive(1)
        go handleClient(s.(*KCPConn))
    }
}

func handleClient(conn *KCPConn) {
    conn.SetNoDelay(1, 20, 2, 1)
    conn.SetWindowSize(1024, 1024) // faster
    conn.SetReadDeadline(time.Now().Add(time.Hour))
    conn.SetWriteDeadline(time.Now().Add(time.Hour))
    //fmt.Println("new client", conn.RemoteAddr())
    buf := make([]byte, 65536)
    count := 0
    for {
        n, err := conn.Read(buf)
        if err != nil {
            panic(err)
        }
        if n == 0 {
            break
        }
        count++
        conn.Write(buf[:n])
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
        go client(&wg)
    }
    wg.Wait()
}

func client(wg *sync.WaitGroup) {
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

/*
This test case is wrong. we should never echo too much data (without read) to make the connection blocked
func TestBigPacket(t *testing.T) {
    var wg sync.WaitGroup
    wg.Add(1)
    go client2(&wg)
    wg.Wait()
}
*/
func client2(wg *sync.WaitGroup) {
    cli, err := DialTest()
    if err != nil {
        panic(err)
    }
    cli.SetNoDelay(1, 20, 2, 1)
    const N = 10
    buf := make([]byte, 1024*512)
    msg := make([]byte, 1024*512)
    for i := 0; i < N; i++ {
        cli.Write(msg)
    }
    println("total written:", len(msg)*N)

    nrecv := 0
    cli.SetReadDeadline(time.Now().Add(3 * time.Second))
    for {
        n, err := cli.Read(buf)
        if err != nil {
            break
        } else {
            nrecv += n
            if nrecv == len(msg)*N {
                break
            }
        }
    }

    println("total recv:", nrecv)
    cli.Close()
    wg.Done()
}

func TestSpeed(t *testing.T) {
    var wg sync.WaitGroup
    wg.Add(1)
    go client3(&wg)
    wg.Wait()
}


func client3(wg *sync.WaitGroup) {
    cli, err := DialTest()
    if err != nil {
        panic(err)
    }
    log.Println("remote:", cli.RemoteAddr(), "local:", cli.LocalAddr())
    log.Println("conv:", cli.GetConv())
    cli.SetWindowSize(1024, 1024)
    cli.SetNoDelay(1, 20, 2, 1)
    start := time.Now()

    msgSize := 4096
    count := 4096 * 10
    go func() {
        buf := make([]byte, 1024*1024)
        nrecv := 0
        for {
            n, err := cli.Read(buf)
            if err != nil {
                fmt.Println(err)
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
        time := time.Now().Sub(start)
        fmt.Printf("time for %.2f MB : %v, speed=%.2f MB/s\n", mb, time, mb / time.Seconds())
        fmt.Printf("%+v\n", Stats)
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
    fmt.Println("testing parallel", par, "connections")
    for i := 0; i < par; i++ {
        go client4(i, &wg)
    }
    wg.Wait()
    fmt.Println("testing parallel", par, "connections done")
}

func client4(idx int, wg *sync.WaitGroup) {
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
