//Package modbusone provides a Modbus library to implement both server and client
//using one set of APIs.
//
//For sample code, see examples/memory, and handler2serial_test.go
package modbusone

import (
	"fmt"
)

//Server is the common interface for all Clients and Servers that use ProtocalHandlers
type Server interface {
	Serve(handler ProtocalHandler) error
}

//ProtocalHandler handles PDUs based on if it is a write or read from the local
//perspective.
type ProtocalHandler interface {
	//OnInput is called on the server for a write request,
	//or on the client for read reply.
	//For write to server on server side, data is part of req.
	//For read from server on client side, req is the req from client, and
	//data is part of reply.
	OnWrite(req PDU, data []byte) error

	//OnOutput is called on the server for a read request,
	//or on the client before write requst.
	//For read from server on the server side, req is from client and data is
	//part of reply.
	//For write to server on the client side, req is from local action
	//(such as RTUClient.StartTransaction), and data will be added to req to send
	//to server.
	OnRead(req PDU) (data []byte, err error)

	//OnError is called on the client when it receive a well formed
	//error from server
	OnError(req PDU, errRep PDU)
}

//FunctionCode Modebus function codes
type FunctionCode byte

//Implemented FunctionCodes
const (
	FcReadCoils              FunctionCode = 1
	FcReadDiscreteInputs     FunctionCode = 2
	FcReadHoldingRegisters   FunctionCode = 3
	FcReadInputRegisters     FunctionCode = 4
	FcWriteSingleCoil        FunctionCode = 5
	FcWriteSingleRegister    FunctionCode = 6
	FcWriteMultipleCoils     FunctionCode = 15
	FcWriteMultipleRegisters FunctionCode = 16
	//FcMaskWriteRegister          FunctionCode = 22
	//FcReadWriteMultipleRegisters FunctionCode = 23
	//FcReadFIFOQueue              FunctionCode = 24 //not supported for now
)

//Valid test if FunctionCode is a supported function, and not an error response
func (f FunctionCode) Valid() bool {
	return (f > 0 && f < 7) || (f > 14 && f < 17) //|| (f > 21 && f < 24)
}

//MaxRange is the largest address in the Modbus protocol.
func (f FunctionCode) MaxRange() uint16 {
	return 0xFFFF
}

//MaxPerPacket returns the max number of values a FunctionCode can carry.
func (f FunctionCode) MaxPerPacket() uint16 {
	switch f {
	case FcReadCoils, FcReadDiscreteInputs:
		return 2000
	case FcReadHoldingRegisters, FcReadInputRegisters:
		return 125 //0x007D
	case FcWriteSingleCoil, FcWriteSingleRegister:
		return 1
	case FcWriteMultipleCoils:
		return 0x07B0 //1968
	case FcWriteMultipleRegisters:
		return 0x007B
	}
	return 0 //unsupported functions
}

//MaxPerPacketSized returns the max number of values a FunctionCode can carry,
//if we are to further limit PDU packet size.
//At least 1 (8 for bools) is returned if size is too small.
func (f FunctionCode) MaxPerPacketSized(size uint8) uint16 {
	if size > MaxPDUSize {
		size = MaxPDUSize
	}
	s := uint16(size)
	switch f {
	case FcReadCoils, FcReadDiscreteInputs:
		if s < 4 {
			return 8
		}
		if s == MaxPDUSize {
			//one byte is not used even at max
			s--
		}
		q := (s - 2) * 8
		return q
	case FcReadHoldingRegisters, FcReadInputRegisters:
		if s < 6 {
			return 1
		}
		return (s - 2) / 2
	case FcWriteSingleCoil, FcWriteSingleRegister:
		return 1
	case FcWriteMultipleCoils:
		if s < 8 {
			return 8
		}
		if s == MaxPDUSize {
			s--
		}
		q := (s - 6) * 8
		return q
	case FcWriteMultipleRegisters:
		if s < 10 {
			return 1
		}
		return (s - 6) / 2
	}
	return 0 //unsupported functions
}

//MakeRequestHeader makes a particular pdu without any data, to be used for
//client side StartTransaction.
//The inverse functions are PDU.GetFunctionCode() .GetAddress() and .GetRequestCount()
func (f FunctionCode) MakeRequestHeader(address, quantity uint16) (PDU, error) {
	if quantity > f.MaxPerPacket() {
		return nil, fmt.Errorf("%v can not pack %v at once", f, quantity)
	}
	if uint32(address)+uint32(quantity) > uint32(f.MaxRange()) {
		return nil, fmt.Errorf("%v + %v out of range %v", address, quantity-1, f.MaxRange())
	}
	header := []byte{byte(f), byte(address >> 8), byte(address)}
	if f.IsSingle() {
		return PDU(header), nil
	}
	header = append(header, byte(quantity>>8), byte(quantity))
	if f == FcWriteMultipleCoils {
		return PDU(append(header, byte((quantity+7)/8))), nil
	}
	if f == FcWriteMultipleRegisters {
		return PDU(append(header, byte(quantity*2))), nil
	}
	return PDU(header), nil
}

//IsUint16 returns true if the FunctionCode concerns 16bit values
func (f FunctionCode) IsUint16() bool {
	switch f {
	case 3, 4, 6, 16:
		return true
	}
	return false
}

//IsBool returns true if the FunctionCode concerns boolean values
func (f FunctionCode) IsBool() bool {
	switch f {
	case 1, 2, 5, 15:
		return true
	}
	return false
}

//IsSingle returns true if the FunctionCode can transmit only one value
func (f FunctionCode) IsSingle() bool {
	switch f {
	case 5, 6:
		return true
	}
	return false
}

//IsWriteToServer returns true if the FunctionCode is a write.
//FunctionCode 23 is both a read and write.
func (f FunctionCode) IsWriteToServer() bool {
	switch f {
	case 5, 6, 15, 16, 22, 23:
		return true
	}
	return false
}

//IsReadToServer returns true if the FunctionCode is a write.
//FunctionCode 23 is both a read and write.
func (f FunctionCode) IsReadToServer() bool {
	switch f {
	case 1, 2, 3, 4, 23:
		return true
	}
	return false
}

//SeparateError test if FunctionCode is an error response, and also return the version
//without error flag set
func (f FunctionCode) SeparateError() (bool, FunctionCode) {
	return f > 0x7f, f & 0x7f
}

//WithError return a copy of FunctionCode with the error flag set.
func (f FunctionCode) WithError() FunctionCode {
	return f + 0x80
}

//ExceptionCode Modbus exception codes
type ExceptionCode byte

//Defined exception codes, 5 to 11 are not used
const (
	//EcOK is invented for no error
	EcOK ExceptionCode = 0
	//EcInternal is invented for error reading ExceptionCode
	EcInternal                           ExceptionCode = 255
	EcIllegalFunction                    ExceptionCode = 1
	EcIllegalDataAddress                 ExceptionCode = 2
	EcIllegalDataValue                   ExceptionCode = 3
	EcServerDeviceFailure                ExceptionCode = 4
	EcAcknowledge                        ExceptionCode = 5
	EcServerDeviceBusy                   ExceptionCode = 6
	EcMemoryParityError                  ExceptionCode = 8
	EcGatewayPathUnavailable             ExceptionCode = 10
	EcGatewayTargetDeviceFailedToRespond ExceptionCode = 11
)

//Error implements error for ExceptionCode
func (e ExceptionCode) Error() string {
	return fmt.Sprintf("ExceptionCode:0x%02X", byte(e))
}

//ToExceptionCode turns an error into an ExceptionCode (to send in PDU), best
//effort with EcServerDeviceFailure as fail back.
func ToExceptionCode(err error) ExceptionCode {
	if err == nil {
		debugf("ToExceptionCode: unexpected covert nil error to ExceptionCode")
	}
	e, ok := err.(ExceptionCode)
	if ok {
		return e
	}
	if err == ErrFcNotSupported {
		return EcIllegalFunction
	}
	return EcServerDeviceFailure
}

//PDU is the Modbus Protocol Data Unit
type PDU []byte

//ExceptionReplyPacket make a PDU packet to reply to request req with ExceptionCode e
func ExceptionReplyPacket(req PDU, e ExceptionCode) PDU {
	fc := req.GetFunctionCode()
	return PDU([]byte{byte(fc) | 0x80, byte(e)})
}

//MatchPDU returns true if ans is a valid reply to ask, including normal and
//error code replies.
func MatchPDU(ask PDU, ans PDU) bool {
	rf := ask.GetFunctionCode()
	af := ans.GetFunctionCode()
	return rf == af%128
}

//ValidateRequest tests for errors in a received Request PDU packet.
//Use ToExceptionCode to get the ExceptionCode for error.
//Checks for errors 2 and 3 are done in GetRequestValues.
func (p PDU) ValidateRequest() error {
	if !p.GetFunctionCode().Valid() {
		return EcIllegalFunction
	}
	if len(p) < 3 {
		return EcIllegalDataAddress
	}
	return nil
}

//GetFunctionCode returns the function code
func (p PDU) GetFunctionCode() FunctionCode {
	if len(p) <= 0 {
		return FunctionCode(0)
	}
	return FunctionCode(p[0])
}

//GetAddress returns the stating address,
//If PDU is invalid, behavior is undefined (can panic).
func (p PDU) GetAddress() uint16 {
	return uint16(p[1])<<8 | uint16(p[2])
}

//GetRequestCount returns the number of values requested,
//If PDU is invalid (too short), behavior is undefined (can panic).
func (p PDU) GetRequestCount() uint16 {
	if p.GetFunctionCode().IsSingle() {
		return 1
	}
	return uint16(p[3])<<8 | uint16(p[4])
}

//GetRequestValues returns the values in a write request
func (p PDU) GetRequestValues() ([]byte, error) {
	f := p.GetFunctionCode()
	if f == 0 {
		return nil, EcIllegalFunction
	}
	if f.IsSingle() {
		if len(p) != 5 {
			debugf("fc %v got %v pdu bytes, expected 5", p.GetFunctionCode(), len(p))
			return nil, EcIllegalDataValue
		}
		return p[3:], nil
	}
	lb := len(p) - 6
	if lb < 1 {
		debugf("fc %v got %v pdu bytes, expected > 6", p.GetFunctionCode(), len(p))
		return nil, EcIllegalDataValue
	}
	if lb != int(p[5]) {
		debugf("decleared %v bytes of data, but got %v bytes", p[5], lb)
		return nil, EcIllegalDataValue
	}
	l := int(p.GetRequestCount())
	// check if start + count is highter than max range
	if l+int(p.GetAddress()) > int(p.GetFunctionCode().MaxRange()) {
		debugf("address out of range")
		return nil, EcIllegalDataAddress
	}
	if f.IsUint16() {
		//16 bits registers
		if lb != l*2 {
			debugf("%v registers does not fit in %v bytes", l, lb)
			return nil, EcIllegalDataValue
		}
	} else {
		//bools
		if lb != (l+7)/8 {
			debugf("%v bools does not fit in %v bytes", l, lb)
			return nil, EcIllegalDataValue
		}
	}
	return p[6:], nil
}

//GetReplyValues returns the values in a read reply
func (p PDU) GetReplyValues() ([]byte, error) {
	l := len(p) - 2 //bytes of values
	if l < 1 || l != int(p[1]) {
		return nil, fmt.Errorf("length mismatch with bytes")
	}
	return p[2:], nil
}

//MakeReadReply produces the reply PDU based on the request PDU and read data
func (p PDU) MakeReadReply(data []byte) PDU {
	return PDU(append([]byte{byte(p.GetFunctionCode()), byte(len(data))}, data...))
}

//MakeWriteRequest produces the request PDU based on the request PDU header and
//(locally) read data
func (p PDU) MakeWriteRequest(data []byte) PDU {
	fc := p.GetFunctionCode()
	switch fc {
	case FcWriteSingleCoil, FcWriteSingleRegister:
		return append(p[:3], data...)
	case FcWriteMultipleCoils, FcWriteMultipleRegisters:
		return append(p[:6], data...)
	}
	debugf("MakeRequestData unsupported for %v\n", fc)
	return nil
}

//MakeWriteReply assumes the request is a successful write, and make the associated response
func (p PDU) MakeWriteReply() PDU {
	if len(p) > 5 {
		return p[:5] //works for 5,6,15,16
	}
	return p
}
