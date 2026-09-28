package protocol

const (
	MsgPing             uint8 = 0x01
	MsgPong             uint8 = 0x02
	MsgFindNodeRequest  uint8 = 0x03
	MsgFindNodeResponse uint8 = 0x04
	MsgError            uint8 = 0x7F
)
