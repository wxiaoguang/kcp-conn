我是光年实验室高级招聘经理。
我在github上访问了你的开源项目，你的代码超赞。你最近有没有在看工作机会，我们在招软件开发工程师，拉钩和BOSS等招聘网站也发布了相关岗位，有公司和职位的详细信息。
我们公司在杭州，业务主要做流量增长，是很多大型互联网公司的流量顾问。公司弹性工作制，福利齐全，发展潜力大，良好的办公环境和学习氛围。
公司官网是http://www.gnlab.com,公司地址是杭州市西湖区古墩路紫金广场B座，若你感兴趣，欢迎与我联系，
电话是0571-88839161，手机号：18668131388，微信号：echo 'bGhsaGxoMTEyNAo='|base64 -D ,静待佳音。如有打扰，还请见谅，祝生活愉快工作顺利。

# kcp-conn

A C++/Go KCP implement.

KCP C++ is driven by libevent, which has the same abstraction as the TCP connection.

Differences from the official version:

1. supports CONNECT/CLOSE
2. FEC/Cipher are removed, and the KCPConn implement is (nearly) completely rewritten.
3. lossy channel in test code supports bandwidth limit, delay and loss ratio.

This project is for special usage, there are only few documents right now. I will try to improve it if I have enough time.

For who is looking for the official KCP, please visit:
* https://github.com/skywind3000/kcp
* https://github.com/xtaci/kcp-go

Feel free to tell me if you have questions or find bugs.
