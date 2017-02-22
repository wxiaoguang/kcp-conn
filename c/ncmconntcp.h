#pragma once

#include "ncmconn.h"

class NcmConnTcp : public NcmConn {
private:
    class Internal;
    Internal *internal;
protected:
    virtual void doClose() override;

public:
    NcmConnTcp(struct event_base *evbase);
    ~NcmConnTcp();

    void accept(int fd);
    virtual void connectAsync(const char *ipPort, int timeout) override;
    virtual void readAsync() override;
    virtual void writeAsync() override;
};
