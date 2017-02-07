package kcp

import (
    "time"
    "math/rand"
    "fmt"
)

type LossyTrick interface {
    DelayMs() int
    LossRatio() float64
}

func LossyChannel(ch chan interface{}, sz int, nt LossyTrick) chan interface{} {
    q := make([][]interface{}, 5000)
    n := 0

    curMs := time.Now().UnixNano() / int64(time.Millisecond)
    curIdx := 0

    out := make(chan interface{}, sz)

    var total int64
    var loss int64

    go func() {
        var tmr *time.Ticker

        tmr = time.NewTicker(time.Millisecond / 10)
        for ch != nil || n != 0 {
            select {

            case v := <- ch:

                if v == nil {
                    ch = nil
                    continue
                }

                total++

                if rand.Float64() < nt.LossRatio() {
                    loss++
                    continue
                }

                d := nt.DelayMs()
                idx := (curIdx + d) % len(q)
                q[idx] = append(q[idx], v)
                n++

            case <-tmr.C:
                nowMs := time.Now().UnixNano() / int64(time.Millisecond)
                d := int(nowMs - curMs)
                for i := 0; i <= d; i++ {
                    idx := (curIdx + i) % len(q)

                    loop:
                    for j := range q[idx] {
                        v := q[idx][j]
                        select {
                        case out <- v:
                            n--
                        case <-time.After(time.Millisecond * 5):
                            r := len(q[idx]) - j
                            n -= r
                            loss += int64(r)
                            break loop
                        }
                    }
                    q[idx] = q[idx][:0]
                }
                curIdx += d
                curMs = nowMs
            }
        }

        fmt.Printf("LossyChannel closed. total=%d, loss=%d\n", total, loss)
        close(out)
    }()
    return out

}