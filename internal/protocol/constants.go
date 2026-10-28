package protocol

const (
	MsgPing             uint8 = 0x01
	MsgPong             uint8 = 0x02
	MsgFindNodeRequest  uint8 = 0x03
	MsgFindNodeResponse uint8 = 0x04

	// Новые типы для этапа 3 (DHT Storage)
	MsgStoreRequest      uint8 = 0x05
	MsgStoreResponse     uint8 = 0x06
	MsgFindValueRequest  uint8 = 0x07
	MsgFindValueResponse uint8 = 0x08

	MsgError uint8 = 0x7F
)
