#pragma once

#include "ncmconn.h"

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

    virtual void connectAsync(const char *ipPort) override;
    virtual void readAsync() override;
    virtual void writeAsync() override;
};
