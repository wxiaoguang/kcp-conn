#pragma once

#include "ncmconn.h"
#include <stdint.h>

class NcmConnKcp;

class NcmConnKcpManager {
    friend class NcmConnKcp;
private:
    class Internal;
    Internal *internal;

public:
    NcmConnKcpManager(struct event_base *evbase);
    ~NcmConnKcpManager();
    NcmConnKcp *newKcpConn();
    void setCloseWaitMs(uint32_t n);
};

class NcmConnKcp : public NcmConn {
    friend class NcmConnKcpManager;

private:
    class Internal;
    Internal *internal;

protected:
    virtual void doClose() override;
    NcmConnKcp(NcmConnKcpManager *manager, uint32_t conversationId);

public:
    ~NcmConnKcp();
    void setKcpMtu(int mtu);
    void setKcpOptions(int noDelay, int interval, int resend, int noCwnd);
    void setKcpWnd(int sndWnd, int rcvWnd);

    virtual void connectAsync(const char *ipPort, int timeout) override;
    virtual void readAsync() override;
    virtual void writeAsync() override;
};
