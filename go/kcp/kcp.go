// Package kcp - A Fast and Reliable ARQ Protocol
package kcp

import (
    "encoding/binary"
    "sync/atomic"
    "log"
    "math"
)

const (
    IKCP_RTO_NDL     = 30  // no delay min rto
    IKCP_RTO_MIN     = 100 // normal min rto
    IKCP_RTO_DEF     = 200
    IKCP_RTO_MAX     = 60000

    IKCP_CMD_CONNECT = 80 // cmd: connect
    IKCP_CMD_PUSH    = 81 // cmd: push data
    IKCP_CMD_ACK     = 82 // cmd: ack
    IKCP_CMD_WASK    = 83 // cmd: window probe (ask)
    IKCP_CMD_WINS    = 84 // cmd: window size (tell)

    IKCP_ASK_SEND    = 1  // need to send IKCP_CMD_WASK
    IKCP_ASK_TELL    = 2  // need to send IKCP_CMD_WINS
    IKCP_WND_SND     = 32
    IKCP_WND_RCV     = 32

    IKCP_MTU_DEF     = 1400
    IKCP_INTERVAL    = 100
    IKCP_OVERHEAD    = 24
    IKCP_DEADLINK    = 20
    IKCP_PROBE_INIT  = 7000   // 7 secs to probe window size
    IKCP_PROBE_LIMIT = 120000 // up to 120 secs to probe window

    IKCP_STATE_CONNECTED        uint32 = 1 << 0
    IKCP_STATE_REMOTE_CLOSED    uint32 = 1 << 1
    IKCP_STATE_LOCAL_CLOSED     uint32 = 1 << 2
    IKCP_STATE_DEAD             uint32 = 1 << 3

    IKCP_STATE_SPEED_NORMAL     uint8 = 0
    IKCP_STATE_SPEED_TRY        uint8 = 1 << 1
    IKCP_STATE_SPEED_DRAIN      uint8 = 1 << 2
    IKCP_STATE_SPEED_RTT        uint8 = 1 << 3
    IKCP_STATE_SPEED_DATA       uint8 = 1 << 4
    IKCP_STATE_SPEED_CHECK_MAX  uint8 = 1 << 5
    IKCP_STATE_SPEED_CHECK_MIN  uint8 = 1 << 6

    IKCP_WND_SPEED_RTT          uint8 = 4
    MAX_UINT32                  uint32 = ^uint32(0)
    CWND_INCREASE_FACTOR        uint32 = 2
    CWND_DECREASE_FACTOR        float32 = 0.75
    RATE_RANGE                  float32 = 0.1
    SPEEDMODEL_TIME_RATE        float32 = 0.02
)

// Output is a closure which captures conn and calls conn.Write
type Output func(buf []byte, size int)

/* encode 8 bits unsigned int */
func ikcp_encode8u(p []byte, c byte) []byte {
    p[0] = c
    return p[1:]
}

/* decode 8 bits unsigned int */
func ikcp_decode8u(p []byte, c *byte) []byte {
    *c = p[0]
    return p[1:]
}

/* encode 16 bits unsigned int (lsb) */
func ikcp_encode16u(p []byte, w uint16) []byte {
    binary.LittleEndian.PutUint16(p, w)
    return p[2:]
}

/* decode 16 bits unsigned int (lsb) */
func ikcp_decode16u(p []byte, w *uint16) []byte {
    *w = binary.LittleEndian.Uint16(p)
    return p[2:]
}

/* encode 32 bits unsigned int (lsb) */
func ikcp_encode32u(p []byte, l uint32) []byte {
    binary.LittleEndian.PutUint32(p, l)
    return p[4:]
}

/* decode 32 bits unsigned int (lsb) */
func ikcp_decode32u(p []byte, l *uint32) []byte {
    *l = binary.LittleEndian.Uint32(p)
    return p[4:]
}

func _imin_(a, b uint32) uint32 {
    if a <= b {
        return a
    } else {
        return b
    }
}

func _imax_(a, b uint32) uint32 {
    if a >= b {
        return a
    } else {
        return b
    }
}

func _ibound_(lower, middle, upper uint32) uint32 {
    return _imin_(_imax_(lower, middle), upper)
}

func _itimediff(later, earlier uint32) int32 {
    return (int32)(later - earlier)
}

// Segment defines a KCP segment
type segment struct {
    conv     uint32
    cmd      uint32
    frg      uint32
    wnd      uint32
    ts       uint32
    sn       uint32
    una      uint32
    resendts uint32
    rto      uint32
    fastack  uint32
    xmit     uint32
    data     []byte
}

// encode a segment into buffer
func (seg *segment) encode(ptr []byte) []byte {
    ptr = ikcp_encode32u(ptr, seg.conv)
    ptr = ikcp_encode8u(ptr, uint8(seg.cmd))
    ptr = ikcp_encode8u(ptr, uint8(seg.frg))
    ptr = ikcp_encode16u(ptr, uint16(seg.wnd))
    ptr = ikcp_encode32u(ptr, seg.ts)
    ptr = ikcp_encode32u(ptr, seg.sn)
    ptr = ikcp_encode32u(ptr, seg.una)
    ptr = ikcp_encode32u(ptr, uint32(len(seg.data)))
    return ptr
}

// KCP defines a single KCP connection
type KCP struct {
    conv, state                            uint32
    mtu, mss                               int
    snd_una, snd_nxt, rcv_nxt              uint32
    ssthresh                               uint32
    rx_rttval, rx_srtt, rx_rto, rx_minrto  uint32
    snd_wnd, rcv_wnd, rmt_wnd, cwnd, probe uint32
    interval, ts_flush, xmit               uint32
    nodelay, updated                       uint32
    ts_probe, probe_wait                   uint32
    dead_link, incr                        uint32

    fastresend     int32
    nocwnd, stream int32

    snd_queue []segment
    rcv_queue []segment
    snd_buf   []segment
    rcv_buf   []segment

    acklist []ackItem

    buffer []byte
    output Output
    stats *KcpConnStats


    speed_state uint8

    min_rtt uint32
    touch_min_rtt bool
    speed_start_ts uint32
    speed_next_start_ts uint32

    wnd_lowbound_speed float32
    wnd_lowbound       uint32
    wnd_upbound_speed  float32
    wnd_upbound        uint32
    base_speed float32
    base_speed_wnd uint32
    current_ack_seg_cnt uint32

    remote_speed_end_ts uint32
    remote_speed_seg_cnt_in_rtt uint32

    drain_begin_ts uint32
    drain_end_ts uint32
    cwnd_before_drain uint32
    inflight uint32
}

type ackItem struct {
    sn uint32
    ts uint32
}

// newKCP create a new kcp control object, 'conv' must equal in two endpoint
// from the same connection.
func newKCP() *KCP {
    kcp := new(KCP)
    kcp.snd_wnd = IKCP_WND_SND
    kcp.rcv_wnd = IKCP_WND_RCV
    kcp.rmt_wnd = IKCP_WND_RCV
    kcp.mtu = IKCP_MTU_DEF
    kcp.mss = kcp.mtu - IKCP_OVERHEAD
    kcp.buffer = make([]byte, (kcp.mtu+IKCP_OVERHEAD)*3)
    kcp.rx_rto = IKCP_RTO_DEF
    kcp.rx_minrto = IKCP_RTO_MIN
    kcp.interval = IKCP_INTERVAL
    kcp.ts_flush = IKCP_INTERVAL
    kcp.dead_link = IKCP_DEADLINK

    kcp.speed_state = IKCP_STATE_SPEED_NORMAL
    kcp.cwnd = IKCP_WND_SND

    kcp.base_speed_wnd = 0
    kcp.base_speed = 0

    kcp.speed_next_start_ts = 0
    kcp.min_rtt = MAX_UINT32
    return kcp
}

// newSegment creates a KCP segment
func (kcp *KCP) newSegment(size int) *segment {
    seg := new(segment)
    seg.data = udpPacketPool.Get().([]byte)[:size]
    return seg
}

// delSegment recycles a KCP segment
func (kcp *KCP) delSegment(seg *segment) {
    udpPacketPool.Put(seg.data)
}

// PeekSize checks the size of next message in the recv queue
func (kcp *KCP) recvSize() (length int) {
    if len(kcp.rcv_queue) == 0 {
        return 0
    }

    seg := &kcp.rcv_queue[0]
    if seg.frg == 0 {
        length = len(seg.data)
        if length == 0 {
            kcp.state |= IKCP_STATE_REMOTE_CLOSED
        }
        return
    }

    if len(kcp.rcv_queue) < int(seg.frg+1) {
        return -1
    }

    for k := range kcp.rcv_queue {
        seg := &kcp.rcv_queue[k]
        length += len(seg.data)
        if seg.frg == 0 {
            break
        }
    }
    return
}

// Recv is user/upper level recv: returns size, returns below zero for EAGAIN
func (kcp *KCP) recv(buffer []byte) (n int) {

    var fast_recover bool
    if len(kcp.rcv_queue) >= int(kcp.rcv_wnd) {
        fast_recover = true
    }

    // merge fragment
    count := 0
    for k := range kcp.rcv_queue {
        seg := &kcp.rcv_queue[k]
        copy(buffer, seg.data)
        buffer = buffer[len(seg.data):]
        n += len(seg.data)
        count++
        kcp.delSegment(seg)
        if seg.frg == 0 {
            break
        }
    }
    kcp.rcv_queue = kcp.rcv_queue[count:]

    // move available data from rcv_buf -> rcv_queue
    kcp.appendRcvQueue()
    // fast recover
    if len(kcp.rcv_queue) < int(kcp.rcv_wnd) && fast_recover {
        // ready to send back IKCP_CMD_WINS in ikcp_flush
        // tell remote my window size
        kcp.probe |= IKCP_ASK_TELL
    }
    return
}

func (kcp *KCP) appendRcvQueue() {
    count := 0
    for k := range kcp.rcv_buf {
        seg := kcp.rcv_buf[k]
        if seg.sn == kcp.rcv_nxt && len(kcp.rcv_queue) < int(kcp.rcv_wnd) {
            kcp.rcv_nxt++
            count++
            if seg.cmd == IKCP_CMD_PUSH {
                kcp.rcv_queue = append(kcp.rcv_queue, seg)
            }
        } else {
            break
        }
    }
    kcp.rcv_buf = kcp.rcv_buf[count:]
}

// Send is user/upper level send, returns below zero for error
func (kcp *KCP) send(buffer []byte) int {
    var count int
    if len(buffer) == 0 {
        return -1
    }

    // append to previous segment in streaming mode (if possible)
    if kcp.stream != 0 {
        n := len(kcp.snd_queue)
        if n > 0 {
            old := &kcp.snd_queue[n-1]
            oldDataLen := len(old.data)
            if oldDataLen < kcp.mss && old.cmd == IKCP_CMD_PUSH {
                capacity := kcp.mss - oldDataLen
                extend := capacity
                if len(buffer) < capacity {
                    extend = len(buffer)
                }
                seg := old
                seg.cmd = old.cmd
                seg.data = seg.data[:len(old.data) + extend]  //from pool, the cap is enough for udp packet
                copy(seg.data[oldDataLen:], buffer)
                buffer = buffer[extend:]
            }
        }

        if len(buffer) == 0 {
            return 0
        }
    }

    count = (len(buffer) + kcp.mss - 1) / kcp.mss

    if kcp.stream == 0 && count > 255 {
        //in message mode, we need to ensure the the count(fragment) is in 8-bit
        return -2
    }

    for i := 0; i < count; i++ {
        var size int
        if len(buffer) > kcp.mss {
            size = kcp.mss
        } else {
            size = len(buffer)
        }
        seg := kcp.newSegment(size)
        seg.cmd = IKCP_CMD_PUSH
        copy(seg.data, buffer[:size])
        if kcp.stream == 0 { // message mode
            seg.frg = uint32(count - i - 1)
        } else { // stream mode
            seg.frg = 0
        }
        kcp.snd_queue = append(kcp.snd_queue, *seg)
        buffer = buffer[size:]
    }
    return 0
}

// optional
func (kcp *KCP) sendConnectFlush(current uint32) {
    if kcp.waitSnd() == 0 && kcp.snd_nxt == 0 {
        seg := kcp.newSegment(0)
        seg.cmd = IKCP_CMD_CONNECT
        kcp.snd_queue = append(kcp.snd_queue, *seg)
        kcp.flush(current)
    }
}

// optional
func (kcp *KCP) sendCloseFlush(current uint32) {
    if kcp.isStateConnected() && !kcp.isStateLocalClosed() {
        kcp.state |= IKCP_STATE_LOCAL_CLOSED
        kcp.state &= ^IKCP_STATE_CONNECTED

        seg := kcp.newSegment(0)
        seg.cmd = IKCP_CMD_PUSH
        kcp.snd_queue = append(kcp.snd_queue, *seg)
        kcp.flush(current)
    }
}

func (kcp *KCP) update_ack(rtt int32) {
    // https://tools.ietf.org/html/rfc6298
    var rto uint32
    if kcp.rx_srtt == 0 {
        kcp.rx_srtt = uint32(rtt)
        kcp.rx_rttval = uint32(rtt) / 2
    } else {
        delta := rtt - int32(kcp.rx_srtt)
        if delta < 0 {
            delta = -delta
        }
        kcp.rx_rttval = (3*kcp.rx_rttval + uint32(delta)) / 4
        kcp.rx_srtt = (7*kcp.rx_srtt + uint32(rtt)) / 8
        if kcp.rx_srtt < 1 {
            kcp.rx_srtt = 1
        }
    }
    rto = kcp.rx_srtt + _imax_(kcp.interval, 4*kcp.rx_rttval)
    kcp.rx_rto = _ibound_(kcp.rx_minrto, rto, IKCP_RTO_MAX)
}

func (kcp *KCP) shrink_buf() {
    if len(kcp.snd_buf) > 0 {
        seg := &kcp.snd_buf[0]
        kcp.snd_una = seg.sn
    } else {
        kcp.snd_una = kcp.snd_nxt
    }
}

func (kcp *KCP) parse_ack(sn uint32) {
    if _itimediff(sn, kcp.snd_una) < 0 || _itimediff(sn, kcp.snd_nxt) >= 0 {
        return
    }

    for k := range kcp.snd_buf {
        seg := &kcp.snd_buf[k]
        if sn == seg.sn {
            kcp.delSegment(seg)
            copy(kcp.snd_buf[k:], kcp.snd_buf[k+1:])
            kcp.snd_buf[len(kcp.snd_buf)-1] = segment{}
            kcp.snd_buf = kcp.snd_buf[:len(kcp.snd_buf)-1]
            break
        }
        if _itimediff(sn, seg.sn) < 0 {
            break
        }
    }
}

func (kcp *KCP) parse_fastack(sn uint32) {
    if _itimediff(sn, kcp.snd_una) < 0 || _itimediff(sn, kcp.snd_nxt) >= 0 {
        return
    }

    for k := range kcp.snd_buf {
        seg := &kcp.snd_buf[k]
        if _itimediff(sn, seg.sn) < 0 {
            break
        } else if sn != seg.sn { //  && kcp.current >= seg.ts+kcp.rx_srtt {
            seg.fastack++
        }
    }
}

func (kcp *KCP) parse_una(una uint32) {
    count := 0
    for k := range kcp.snd_buf {
        seg := &kcp.snd_buf[k]
        if _itimediff(una, seg.sn) > 0 {
            kcp.delSegment(seg)
            count++
        } else {
            break
        }
    }
    kcp.snd_buf = kcp.snd_buf[count:]
}

// ack append
func (kcp *KCP) ack_push(sn, ts uint32) {
    kcp.acklist = append(kcp.acklist, ackItem{sn: sn, ts: ts})
}

func (kcp *KCP) parse_data(newseg *segment) {
    sn := newseg.sn
    if _itimediff(sn, kcp.rcv_nxt+kcp.rcv_wnd) >= 0 ||
        _itimediff(sn, kcp.rcv_nxt) < 0 {
        kcp.delSegment(newseg)
        return
    }

    n := len(kcp.rcv_buf) - 1
    insert_idx := 0
    repeat := false
    for i := n; i >= 0; i-- {
        seg := &kcp.rcv_buf[i]
        if seg.sn == sn {
            repeat = true
            kcp.stats.SegRepeat++
            break
        }
        if _itimediff(sn, seg.sn) > 0 {
            insert_idx = i + 1
            break
        }
    }

    if !repeat {
        if insert_idx == n+1 {
            kcp.rcv_buf = append(kcp.rcv_buf, *newseg)
        } else {
            kcp.rcv_buf = append(kcp.rcv_buf, segment{})
            copy(kcp.rcv_buf[insert_idx+1:], kcp.rcv_buf[insert_idx:])
            kcp.rcv_buf[insert_idx] = *newseg
        }
    } else {
        kcp.delSegment(newseg)
    }

    // move available data from rcv_buf -> rcv_queue
    kcp.appendRcvQueue()
}

// Input when you received a low level packet (eg. UDP packet), call it
func (kcp *KCP) input(current uint32, data []byte, update_ack bool) int {
    if len(kcp.rcv_queue) > 1024 {
        log.Printf("rcv_queue > 1024 conv: %d len:%d", kcp.conv, len(kcp.rcv_queue))
    }
    if len(data) < IKCP_OVERHEAD {
        return -1
    }

    var maxack uint32
    var recentack uint32
    var flag int

    for {
        var ts, sn, length, una, conv uint32
        var wnd uint16
        var cmd, frg uint8

        if len(data) < int(IKCP_OVERHEAD) {
            break
        }

        data = ikcp_decode32u(data, &conv)
        if conv != kcp.conv {
            return -1
        }

        data = ikcp_decode8u(data, &cmd)
        data = ikcp_decode8u(data, &frg)
        data = ikcp_decode16u(data, &wnd)
        data = ikcp_decode32u(data, &ts)
        data = ikcp_decode32u(data, &sn)
        data = ikcp_decode32u(data, &una)
        data = ikcp_decode32u(data, &length)
        if len(data) < int(length) {
            return -2
        }

        if cmd != IKCP_CMD_PUSH && cmd != IKCP_CMD_ACK &&
            cmd != IKCP_CMD_WASK && cmd != IKCP_CMD_WINS &&
            cmd != IKCP_CMD_CONNECT {
            return -3
        }

        //log.Printf("conv: %d cmd: %d frg: %d wnd: %d ts: %d sn: %d una: %d", conv, cmd, frg, wnd, ts, sn, una)

        kcp.rmt_wnd = uint32(wnd)
        kcp.parse_una(una)
        kcp.shrink_buf()

        if cmd == IKCP_CMD_CONNECT {
            if kcp.rcv_nxt == 0 {
            kcp.ack_push(sn, ts)
            kcp.state |= IKCP_STATE_CONNECTED
            }
        } else if cmd == IKCP_CMD_ACK {
            kcp.calc_min_rtt(sn, ts, current)

            kcp.parse_ack(sn)
            kcp.shrink_buf()
            if flag == 0 {
                flag = 1
                maxack = sn
            } else if _itimediff(sn, maxack) > 0 {
                maxack = sn
            }
            recentack = ts
        } else if cmd == IKCP_CMD_PUSH {
            if _itimediff(sn, kcp.rcv_nxt+kcp.rcv_wnd) < 0 {
                if _itimediff(sn, kcp.rcv_nxt) >= 0 {
                    kcp.ack_push(sn, ts)
                    seg := kcp.newSegment(int(length))
                    seg.conv = conv
                    seg.cmd = uint32(cmd)
                    seg.frg = uint32(frg)
                    seg.wnd = uint32(wnd)
                    seg.ts = ts
                    seg.sn = sn
                    seg.una = una
                    copy(seg.data, data[:length])
                    kcp.parse_data(seg)
                } else {
                    kcp.stats.SegRepeat ++
                }
            } else {
                kcp.stats.SegRepeat ++
            }
        } else if cmd == IKCP_CMD_WASK {
            // ready to send back IKCP_CMD_WINS in Ikcp_flush
            // tell remote my window size
            kcp.probe |= IKCP_ASK_TELL
        } else if cmd == IKCP_CMD_WINS {
            // do nothing
        } else {
            return -3
        }

        data = data[length:]
    }

    kcp.updateSpeedState(current)

    if flag != 0 && update_ack {
        kcp.parse_fastack(maxack)
        if _itimediff(current, recentack) >= 0 {
            kcp.update_ack(_itimediff(current, recentack))
        }
    }

    kcp.calc_inflight()
    return 0
}

func (kcp *KCP) calc_min_rtt(sn uint32, ts uint32, current uint32) {
    rtt := current - ts
    if rtt <=0 {
        log.Printf("this can't happen current:%d ts:%d rtt:%d", current, ts, rtt)
        return
    }

    if kcp.min_rtt > rtt {
        kcp.min_rtt = rtt
        if kcp.isInSpeedNormal() {
            kcp.touch_min_rtt = true
        }
    }
}

func (kcp *KCP) wnd_unused() int32 {
    if len(kcp.rcv_queue) < int(kcp.rcv_wnd) {
        return int32(int(kcp.rcv_wnd) - len(kcp.rcv_queue))
    }
    return 0
}

func (kcp *KCP) sndBufAvail() int32 {
    cwnd := kcp.calc_cwnd()
    return _itimediff(cwnd, kcp.inflight)
}

func (kcp *KCP) calc_cwnd() uint32 {
    return kcp.cwnd
}

// flush pending data
func (kcp *KCP) flush(current uint32) {

    kcp.updateSpeedState(current)

    if len(kcp.snd_buf) > 1024 {
        log.Printf("snd_buf > 1024 conv: %d len:%d", kcp.conv, len(kcp.snd_buf))
    }
    buffer := kcp.buffer
    change := 0

    var seg segment
    seg.conv = kcp.conv
    seg.cmd = IKCP_CMD_ACK
    seg.wnd = uint32(kcp.wnd_unused())
    seg.una = kcp.rcv_nxt

    // flush acknowledges
    ptr := buffer
    for i, ack := range kcp.acklist {
        size := len(buffer) - len(ptr)
        if size+IKCP_OVERHEAD > kcp.mtu {
            kcp.output(buffer, size)
            ptr = buffer
        }
        // filter jitters caused by bufferbloat
        if ack.sn >= kcp.rcv_nxt || len(kcp.acklist)-1 == i {
            seg.sn, seg.ts = ack.sn, ack.ts
            ptr = seg.encode(ptr)
        }
    }
    kcp.acklist = nil

    // probe window size (if remote window size equals zero)
    if kcp.rmt_wnd == 0 {
        if kcp.probe_wait == 0 {
            kcp.probe_wait = IKCP_PROBE_INIT
            kcp.ts_probe = current + kcp.probe_wait
        } else {
            if _itimediff(current, kcp.ts_probe) >= 0 {
                if kcp.probe_wait < IKCP_PROBE_INIT {
                    kcp.probe_wait = IKCP_PROBE_INIT
                }
                kcp.probe_wait += kcp.probe_wait / 2
                if kcp.probe_wait > IKCP_PROBE_LIMIT {
                    kcp.probe_wait = IKCP_PROBE_LIMIT
                }
                kcp.ts_probe = current + kcp.probe_wait
                kcp.probe |= IKCP_ASK_SEND
            }
        }
    } else {
        kcp.ts_probe = 0
        kcp.probe_wait = 0
    }

    // flush window probing commands
    if (kcp.probe & IKCP_ASK_SEND) != 0 {
        seg.cmd = IKCP_CMD_WASK
        size := len(buffer) - len(ptr)
        if size+IKCP_OVERHEAD > kcp.mtu {
            kcp.output(buffer, size)
            ptr = buffer
        }
        ptr = seg.encode(ptr)
    }

    // flush window probing commands
    if (kcp.probe & IKCP_ASK_TELL) != 0 {
        seg.cmd = IKCP_CMD_WINS
        size := len(buffer) - len(ptr)
        if size+IKCP_OVERHEAD > kcp.mtu {
            kcp.output(buffer, size)
            ptr = buffer
        }
        ptr = seg.encode(ptr)
    }

    kcp.probe = 0


    // sliding window, controlled by snd_nxt && sna_una+cwnd
    snd_buf_avail_count := kcp.sndBufAvail()
    count := 0

    for k := range kcp.snd_queue {
        snd_buf_avail_count--
        if snd_buf_avail_count < 0 {
            break
        }
        newseg := kcp.snd_queue[k]
        newseg.conv = kcp.conv
        newseg.wnd = seg.wnd
        newseg.ts = current
        newseg.sn = kcp.snd_nxt
        newseg.una = kcp.rcv_nxt
        newseg.resendts = newseg.ts
        newseg.rto = kcp.rx_rto
        kcp.snd_buf = append(kcp.snd_buf, newseg)
        kcp.snd_nxt++
        count++
        kcp.snd_queue[k].data = nil
    }
    kcp.snd_queue = kcp.snd_queue[count:]

    // flag pending data
    hasPending := false
    if count > 0 {
        hasPending = true
    }

    // calculate resent
    resent := uint32(kcp.fastresend)
    if kcp.fastresend <= 0 {
        resent = 0xffffffff
    }

    // flush data segments
    var lostSegs, fastRetransSegs, earlyRetransSegs int64
    for k := range kcp.snd_buf {
        segment := &kcp.snd_buf[k]
        needsend := false
        if segment.xmit == 0 {
            needsend = true
            segment.xmit++
            segment.rto = kcp.rx_rto
            //segment.resendts = current + segment.rto
            segment.resendts = current + kcp.interval
        } else if _itimediff(current, segment.resendts) >= 0 {
            needsend = true
            segment.xmit++
            kcp.xmit++
            if kcp.nodelay == 0 {
                segment.rto += kcp.rx_rto
            } else {
                segment.rto += kcp.rx_rto / 2
            }
            segment.resendts = current + segment.rto
            if segment.xmit > 2 {
                lostSegs++
                //log.Printf("flush lost because resenddts sn: %d", segment.sn)
            }
        } else if segment.fastack >= resent { // fast retransmit
            //lastsend := segment.resendts - segment.rto
            //if _itimediff(current, lastsend) >= int32(kcp.rx_rto/4) {
                needsend = true
                segment.xmit++
                segment.fastack = 0
                segment.resendts = current + segment.rto
                change++
                fastRetransSegs++
            //log.Printf("flush lost because fastack sn: %d", segment.sn)
            //}
        } else if segment.fastack > 0 && !hasPending { // early retransmit
            lastsend := segment.resendts - segment.rto
            if _itimediff(current, lastsend) >= int32(kcp.rx_rto/4) {
                needsend = true
                segment.xmit++
                segment.fastack = 0
                segment.resendts = current + segment.rto
                change++
                earlyRetransSegs++
            }
        }

        if kcp.isInSpeedRtt() && segment.xmit > 1 && lostSegs == 0 {
            // rtt测速阶段，不快速重传，除非真的丢包
            needsend = false
        }

        if needsend {
            segment.ts = current
            segment.wnd = seg.wnd
            segment.una = kcp.rcv_nxt

            //if beginSeg == nil {
            //    beginSeg = segment
            //    endSeg = segment
            //}

            size := len(buffer) - len(ptr)
            need := IKCP_OVERHEAD + len(segment.data)

            if size+need > kcp.mtu {
                //log.Printf("flush send seg cmd: %d frg: %d wnd: %d ts: %d sn: %d una: %d to cmd: %d frg: %d wnd: %d ts: %d sn: %d una: %d",
                //    beginSeg.cmd, beginSeg.frg, beginSeg.wnd, beginSeg.ts, beginSeg.sn, beginSeg.una,
                //    endSeg.cmd, endSeg.frg, endSeg.wnd, endSeg.ts, endSeg.sn, endSeg.una)
                //beginSeg = segment
                //endSeg = segment
                kcp.output(buffer, size)
                ptr = buffer
            }
            //} else {
            //    endSeg = segment
            //}

            if (segment.cmd == IKCP_CMD_PUSH || segment.cmd == IKCP_CMD_CONNECT) && kcp.stats != nil {
                kcp.stats.SegPush ++
                atomic.AddInt64(&Stats.SegPush, 1)
            }

            ptr = segment.encode(ptr)
            copy(ptr, segment.data)
            ptr = ptr[len(segment.data):]

            /*
            if segment.xmit >= kcp.dead_link {
                kcp.state |= IKCP_STATE_DEAD
            }
            */
        }
    }

    segPushResend := lostSegs + fastRetransSegs + earlyRetransSegs
    kcp.stats.SegPushResend += segPushResend
    kcp.stats.SegPushResendLost += lostSegs
    kcp.stats.SegPushResendFast += fastRetransSegs
    kcp.stats.SegPushResendEarly += earlyRetransSegs
    atomic.AddInt64(&Stats.SegPushResend, segPushResend)

    // flash remain segments
    size := len(buffer) - len(ptr)
    if size > 0 {
        //if beginSeg != nil && endSeg != nil {
        //    log.Printf("flush send seg cmd: %d frg: %d wnd: %d ts: %d sn: %d una: %d to cmd: %d frg: %d wnd: %d ts: %d sn: %d una: %d",
        //        beginSeg.cmd, beginSeg.frg, beginSeg.wnd, beginSeg.ts, beginSeg.sn, beginSeg.una,
        //        endSeg.cmd, endSeg.frg, endSeg.wnd, endSeg.ts, endSeg.sn, endSeg.una)
        //}
        kcp.output(buffer, size)
    }

    kcp.calc_inflight()
}
// Update updates state (call it repeatedly, every 10ms-100ms), or you can ask
// ikcp_check when to call it again (without ikcp_input/_send calling).
// 'current' - current timestamp in millisec.
func (kcp *KCP) update(current uint32) {
    var slap int32

    if kcp.updated == 0 {
        kcp.updated = 1
        kcp.ts_flush = current
    }

    slap = _itimediff(current, kcp.ts_flush)

    if slap >= 10000 || slap < -10000 {
        kcp.ts_flush = current
        slap = 0
    }

    if slap >= 0 {
        kcp.ts_flush += kcp.interval
        if _itimediff(current, kcp.ts_flush) >= 0 {
            kcp.ts_flush = current + kcp.interval
        }
        kcp.flush(current)
    }
}

// Check determines when should you invoke ikcp_update:
// returns when you should invoke ikcp_update in millisec, if there
// is no ikcp_input/_send calling. you can call ikcp_update in that
// time, instead of call update repeatly.
// Important to reduce unnacessary ikcp_update invoking. use it to
// schedule ikcp_update (eg. implementing an epoll-like mechanism,
// or optimize ikcp_update when handling massive kcp connections)
func (kcp *KCP) check(current uint32) uint32 {
    ts_flush := kcp.ts_flush
    tm_flush := int32(0x7fffffff)
    tm_packet := int32(0x7fffffff)
    minimal := uint32(0)
    if kcp.updated == 0 {
        return current
    }

    if _itimediff(current, ts_flush) >= 10000 ||
        _itimediff(current, ts_flush) < -10000 {
        ts_flush = current
    }

    if _itimediff(current, ts_flush) >= 0 {
        return current
    }

    tm_flush = _itimediff(ts_flush, current)

    for k := range kcp.snd_buf {
        seg := &kcp.snd_buf[k]
        diff := _itimediff(seg.resendts, current)
        if diff <= 0 {
            return current
        }
        if diff < tm_packet {
            tm_packet = diff
        }
    }

    minimal = uint32(tm_packet)
    if tm_packet >= tm_flush {
        minimal = uint32(tm_flush)
    }
    if minimal >= kcp.interval {
        minimal = kcp.interval
    }

    return current + minimal
}

// SetMtu changes MTU size, default is 1400
func (kcp *KCP) setMtu(mtu int) int {
    if mtu < 50 || mtu < IKCP_OVERHEAD {
        return -1
    }
    buffer := make([]byte, (mtu+IKCP_OVERHEAD)*3)
    if buffer == nil {
        return -2
    }
    kcp.mtu = mtu
    kcp.mss = kcp.mtu - IKCP_OVERHEAD
    kcp.buffer = buffer
    return 0
}

// NoDelay options
// fastest: ikcp_nodelay(kcp, 1, 20, 2, 1)
// nodelay: 0:disable(default), 1:enable
// interval: internal update timer interval in millisec, default is 100ms
// resend: 0:disable fast resend(default), 1:enable fast resend
// nc: 0:normal congestion control(default), 1:disable congestion control
func (kcp *KCP) setNoDelay(nodelay, interval, resend, nc int) int {
    if nodelay >= 0 {
        kcp.nodelay = uint32(nodelay)
        if nodelay != 0 {
            kcp.rx_minrto = IKCP_RTO_NDL
        } else {
            kcp.rx_minrto = IKCP_RTO_MIN
        }
    }
    if interval >= 0 {
        if interval > 5000 {
            interval = 5000
        } else if interval < 10 {
            interval = 10
        }
        kcp.interval = uint32(interval)
    }
    if resend >= 0 {
        kcp.fastresend = int32(resend)
    }
    if nc >= 0 {
        kcp.nocwnd = int32(nc)
    }
    return 0
}

// WndSize sets maximum window size: sndwnd=32, rcvwnd=32 by default
func (kcp *KCP) setWndSize(sndwnd, rcvwnd int) int {
    if sndwnd > 0 {
        kcp.snd_wnd = uint32(sndwnd)
    }
    if rcvwnd > 0 {
        kcp.rcv_wnd = uint32(rcvwnd)
    }
    return 0
}

// WaitSnd gets how many packet is waiting to be sent
func (kcp *KCP) waitSnd() int {
    return len(kcp.snd_buf) + len(kcp.snd_queue)
}

func (kcp *KCP) isStateConnected() bool {
    return (kcp.state & IKCP_STATE_CONNECTED) != 0
}

func (kcp *KCP) isStateRemoteClosed() bool {
    return (kcp.state & IKCP_STATE_REMOTE_CLOSED) != 0
}

func (kcp *KCP) isStateLocalClosed() bool {
    return (kcp.state & IKCP_STATE_LOCAL_CLOSED) != 0
}

func (kcp *KCP) isStateDead() bool {
    return (kcp.state & IKCP_STATE_DEAD) != 0
}

func (kcp *KCP) isRemoteOpen() bool {
    return (kcp.state & (IKCP_STATE_REMOTE_CLOSED | IKCP_STATE_DEAD)) == 0
}

func (kcp *KCP) isAllOpen() bool {
    return (kcp.state & (IKCP_STATE_REMOTE_CLOSED | IKCP_STATE_LOCAL_CLOSED | IKCP_STATE_DEAD)) == 0
}

func (kcp *KCP) shouldClose() bool {
    return (kcp.state & (IKCP_STATE_REMOTE_CLOSED | IKCP_STATE_LOCAL_CLOSED )) == IKCP_STATE_REMOTE_CLOSED
}

// speed related begin
func (kcp *KCP) startSpeedModel(current uint32) {
    kcp.touch_min_rtt = false
    kcp.speed_next_start_ts = MAX_UINT32
    kcp.speed_start_ts = current
    kcp.inSpeedRtt()
}

func (kcp *KCP) resetSpeedModelStartState(current uint32) {
    kcp.touch_min_rtt = false
    kcp.speed_next_start_ts += kcp.speed_next_start_ts -kcp.speed_start_ts
    kcp.speed_start_ts = current
}

func (kcp *KCP) calc_inflight() {
    kcp.inflight = uint32(len(kcp.snd_buf))
}

func (kcp *KCP) isInSpeedNormal() bool {
    return kcp.speed_state == IKCP_STATE_SPEED_NORMAL
}

func (kcp *KCP) isInSpeedData() bool {
    return kcp.speed_state & IKCP_STATE_SPEED_DATA > 0
}

func (kcp *KCP) isInSpeedDrain() bool {
    return kcp.speed_state & IKCP_STATE_SPEED_DRAIN > 0
}

func (kcp *KCP) isInSpeedTry() bool {
    return kcp.speed_state & IKCP_STATE_SPEED_TRY > 0
}

func (kcp *KCP) isInSpeedRtt() bool {
    return kcp.speed_state & IKCP_STATE_SPEED_RTT > 0
}

func (kcp *KCP) isInSpeedCheckMax() bool {
    return kcp.speed_state & IKCP_STATE_SPEED_CHECK_MAX > 0
}

func (kcp *KCP) isInSpeedCheckMin() bool {
    return kcp.speed_state & IKCP_STATE_SPEED_CHECK_MIN > 0
}

func (kcp *KCP) inSpeedDrain(current uint32) {
    kcp.speed_state |= IKCP_STATE_SPEED_DRAIN
    // drain模式不再向buf里填充数据
    kcp.cwnd_before_drain = kcp.cwnd
    kcp.cwnd = 0
    kcp.drain_begin_ts = current
}

func (kcp *KCP) outSpeedDrain(current uint32) {
    kcp.speed_state &= ^IKCP_STATE_SPEED_DRAIN
    kcp.drain_end_ts = current
}

func (kcp *KCP) inSpeedTry() {
    kcp.speed_state |= IKCP_STATE_SPEED_TRY
    kcp.cwnd = 0
}

func (kcp *KCP) outSpeedTry() {
    kcp.speed_state &= ^IKCP_STATE_SPEED_TRY
}

func (kcp *KCP) inSpeedRtt() {
    kcp.speed_state |= IKCP_STATE_SPEED_RTT
    kcp.cwnd = uint32(IKCP_WND_SPEED_RTT)
}

func (kcp *KCP) outSpeedRtt() {
    kcp.speed_state &= ^IKCP_STATE_SPEED_RTT
    if kcp.base_speed_wnd == 0 && kcp.base_speed == 0 {
        // 第一次初始化
        kcp.base_speed_wnd = uint32(IKCP_WND_SPEED_RTT)
        // 速度是seg/second
        kcp.base_speed = float32(kcp.base_speed_wnd * 1000) / float32(kcp.drain_end_ts - kcp.drain_begin_ts)
    }
}

func (kcp *KCP) inSpeedData() {
    // 进入测速模式，首先检测wnd_upbound
    kcp.speed_state |= IKCP_STATE_SPEED_DATA | IKCP_STATE_SPEED_CHECK_MAX
    kcp.cwnd = kcp.base_speed_wnd * CWND_INCREASE_FACTOR
}

func (kcp *KCP) outSpeedCheckMax() {
    kcp.speed_state &= ^IKCP_STATE_SPEED_CHECK_MAX
}

func (kcp *KCP) inSpeedCheckMin() {
    // 开始检测wnd_lowbound
    kcp.speed_state |= IKCP_STATE_SPEED_CHECK_MIN
    kcp.cwnd = uint32(float32(kcp.cwnd) * CWND_DECREASE_FACTOR)
}

func (kcp *KCP) outSpeedCheckMin() {
    kcp.speed_state &= ^IKCP_STATE_SPEED_CHECK_MIN
}

func (kcp *KCP) outSpeedDate() {
    kcp.speed_state &= ^IKCP_STATE_SPEED_DATA
}

func (kcp *KCP) updateSpeedState(current uint32) {
    if kcp.speed_next_start_ts <= current {
        // 到时间了，看看是不是需要测一轮
        if kcp.touch_min_rtt {
            kcp.resetSpeedModelStartState(current)
        } else {
            kcp.startSpeedModel(current)
        }
    } else if kcp.isInSpeedDrain() && len(kcp.snd_buf) == 0 {
        // 已经排空buffer并且在drain的state
        kcp.outSpeedDrain(current)
        if kcp.isInSpeedTry() {
            // 从try进入rtt测速阶段
            kcp.outSpeedTry()
            kcp.inSpeedRtt()
        } else if kcp.isInSpeedRtt() {
            // rtt测速完成，进入data测速阶段
            kcp.outSpeedRtt()
            kcp.inSpeedData()
        } else {
            // in speed data state
            kcp.updateCwnd(current)
        }
    }
}

func (kcp *KCP) updateCwnd(current uint32) {
    ts := kcp.drain_end_ts - kcp.drain_begin_ts
    current_wnd := kcp.cwnd_before_drain
    current_speed := float32(current_wnd * 1000) / float32(ts)

    // 速度变化率
    diff_speed_rate := (current_speed - kcp.base_speed) / kcp.base_speed

    if kcp.isInSpeedCheckMax() {
        if diff_speed_rate > RATE_RANGE {
            // 速度变快了
            kcp.cwnd = current_wnd * CWND_INCREASE_FACTOR
        } else {
            // 找到max了
            kcp.wnd_upbound = current_wnd
            kcp.wnd_upbound_speed = current_speed
            kcp.outSpeedCheckMax()
            kcp.inSpeedCheckMin()
            kcp.cwnd = kcp.wnd_lowbound + (kcp.wnd_upbound - kcp.wnd_lowbound) / 2
        }
    } else if kcp.isInSpeedCheckMin() {
        if diff_speed_rate > RATE_RANGE {
            // 速度变快了
            kcp.cwnd = uint32(float32(current_wnd) * CWND_DECREASE_FACTOR)
        } else {
            // 找到min了
            kcp.wnd_lowbound = current_wnd
            kcp.wnd_lowbound_speed = current_speed
            kcp.outSpeedCheckMin()
            // max min都测出来了，开始二分
            kcp.cwnd = kcp.wnd_lowbound + (kcp.wnd_upbound - kcp.wnd_lowbound) / 2
        }
    } else {
        // up lowbound都查出来了，开始精细定位了
        if diff_speed_rate > RATE_RANGE {
            if current_wnd > kcp.base_speed_wnd {
                // 处于增长阶段，所以应该继续向着upbound增长
                kcp.cwnd = current_wnd + (kcp.wnd_upbound - current_wnd) / 2
            } else if current_wnd < kcp.base_speed_wnd{
                // 处于减少阶段，所以应该继续向着lowbound减少
                kcp.cwnd = current_wnd - (current_wnd - kcp.wnd_lowbound) / 2
            }
        } else if diff_speed_rate < -RATE_RANGE {
            if current_wnd > kcp.base_speed_wnd {
                // 处于增长阶段，所以找到新的upbound
                kcp.wnd_upbound = current_wnd
                kcp.wnd_upbound_speed = current_speed
                kcp.cwnd = kcp.wnd_lowbound + (kcp.wnd_upbound - kcp.wnd_lowbound) / 2
            } else if current_wnd < kcp.base_speed_wnd{
                // 处于减少阶段，所以找到新的lowbound
                kcp.wnd_lowbound = current_wnd
                kcp.wnd_lowbound_speed = current_speed
                kcp.cwnd = kcp.wnd_lowbound + (kcp.wnd_upbound - kcp.wnd_lowbound) / 2
            }
        } else {
            // 没有变化， 取中间值
            if current_wnd > kcp.base_speed_wnd {
                kcp.cwnd = kcp.base_speed_wnd + (current_wnd - kcp.base_speed_wnd) / 2
            } else if current_wnd < kcp.base_speed_wnd{
                kcp.cwnd = current_wnd - (kcp.base_speed_wnd - current_wnd) / 2
            }
        }

    }

    log.Printf("current_wnd:%d current_speed:%f base_speed:%f base_speed_wnd:%d", current_wnd, current_speed, kcp.base_speed, kcp.base_speed_wnd)
    kcp.base_speed = current_speed
    kcp.base_speed_wnd = current_wnd

    if math.Abs(float64(kcp.cwnd - current_wnd)) <= 1 {
        kcp.stopSpeedModel(current)
    }
}

func (kcp *KCP) stopSpeedModel(current uint32) {
    kcp.speed_state = IKCP_STATE_SPEED_NORMAL
    kcp.speed_next_start_ts = kcp.speed_start_ts + uint32(float32(current - kcp.speed_start_ts) / SPEEDMODEL_TIME_RATE)
    log.Printf("stop speed model: cost:%d next start ts:%d current speed:%f current wnd:%d", current - kcp.speed_start_ts, kcp.speed_next_start_ts, kcp.base_speed, kcp.base_speed_wnd)
}

// speed related end
