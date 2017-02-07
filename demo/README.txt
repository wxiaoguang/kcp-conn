compile kcpconn-demo
go run server.go
./kcpconn-demo

use curl to do the test:
curl -v --proxy http://127.0.0.1:8080 "http://www.xxxxxxxxxxxx.com"
curl -vk --proxy http://127.0.0.1:8080 "https://www.xxxxxxxxxxxx.com"
