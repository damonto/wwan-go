package mbim

import "encoding"

func basicUint32Request(transactionID, cid uint32, commandType CommandType, data []byte, response encoding.BinaryUnmarshaler) *Request {
	return &Request{
		MessageType:   MessageTypeCommand,
		TransactionID: transactionID,
		Timeout:       mbimCIDResponseTimeout,
		Command:       command(ServiceBasicConnect, cid, commandType, data),
		Response:      response,
	}
}
