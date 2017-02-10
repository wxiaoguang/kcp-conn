package kcp

import (
    "testing"
    "time"
    "log"
)


func TestSpeedLossy(t *testing.T) {
    trick1 := &testConnTrick{delayMs: 120, lossRatio: 0.0, limitPerSecond: 1000 * 1024}
    trick2 := &testConnTrick{delayMs: 120, lossRatio: 0.0, limitPerSecond: 1000 * 1024}

    //trick1 = &testConnTrick{delayMs: 0, lossRatio: 0}
    //trick2 = &testConnTrick{delayMs: 0, lossRatio: 0}

    lc1, lc2 := NewLossyPairConn(1024, trick1, trick2)

    c1 := newKCPConn()
    c2 := newKCPConn()
    c1.SetNoDelay(1, 20, 3, 0)
    c1.SetWindowSize(1024, 1024)
    c2.SetNoDelay(1, 20, 3, 0)
    c2.SetWindowSize(1024, 1024)

    chOver := make(chan int, 1)

    msgSize := 4096
    count := 500
    start := time.Now()

    go func() {
        for {
            c1.dump()
            c2.dump()
            time.Sleep(time.Second)
        }
    }()

    go func() {
        c1.connect(0, lc1, nil)
        buf := make([]byte, msgSize)
        for i := 0; i < count; i++ {
            c1.Write(buf)
            log.Println("sendSize", msgSize * i)
        }
    }()

    recvSize := 0
    go func() {
        c2.accept(0, nil, lc2, nil)
        c2.goRunClientRecv() //a fake server should do recv by the conn

        buf := make([]byte, msgSize)
        for {
            n, _ := c2.Read(buf)
            recvSize += n
            if n == 0 || recvSize == msgSize * count {
                break
            }

        }
        c1.Close()
        c2.Close()
        close(chOver)
    }()

    <-chOver

    println("recvSize", recvSize)
    mb := (float64(recvSize)/1024.0/1024.0)
    d := time.Now().Sub(start)
    log.Printf("time for %.2f MB : %v, speed=%.2f MB/s\n", mb, d, mb / d.Seconds())
    log.Printf("Stats: %+v\n", Stats)

    <- time.After(time.Second)
}

