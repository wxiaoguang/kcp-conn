package kcp

type KcpGlobalStats struct {
	SegPush            int64
	SegPushResend      int64

	TotalAccept  int64
	TotalConnect int64
	TotalClose int64
	TotalCloseDangling int64

	ConnMax      int64
	ConnCurrent  int64
	ConnClosing  int64

	ErrorRead    int64
	ErrorInput   int64
	ErrorOutput  int64

	ByteRx       int64
	ByteTx       int64

	PacketIn     int64
	PacketOut    int64
	ByteIn 		 int64
	ByteOut 	 int64

}

type KcpConnStats struct {
	ErrorRead    int64
	ErrorInput   int64
	ErrorOutput  int64

	ByteRx             int64
	ByteTx             int64

	PacketIn           int64
	PacketOut          int64
	ByteIn 		 int64
	ByteOut 	 int64

	SegRepeat      	   int64
	SegPush            int64
	SegPushResend      int64
	SegPushResendFast  int64
	SegPushResendLost  int64
	SegPushResendEarly int64

}

var Stats KcpGlobalStats