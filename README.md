# kcp-conn

A C++/Go KCP implement.

KCP C++ is driven by libevent, which has the same abstraction as the TCP connection.

Differences from the official version:
1. supports CONNECT/CLOSE
2. FEC/Cipher are removed, and the KCPConn implement is (nearly) completely rewritten.
3. lossy channel in test code supports bandwidth limit, delay and loss ratio.

This project is for special usage, there are only few documents right now.
I will try to improve it if I have enough time.

For who is looking for the official KCP, please visit:
https://github.com/skywind3000/kcp
https://github.com/xtaci/kcp-go

Feel free to tell me if you have questions or find bugs.
