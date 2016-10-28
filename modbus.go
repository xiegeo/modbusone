package modbusone

import (
	"fmt"
)

type Server interface {
	Serve(handler ProtocalHandler) error
}

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

//Modebus function codes
type FunctionCode byte

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

//Valid test if FunctionCode is a allowed function, and not an error response
func (f FunctionCode) Valid() bool {
	return (f > 0 && f < 7) || (f > 14 && f < 17) //|| (f > 21 && f < 24)
}

func (f FunctionCode) MaxRange() uint16 {
	return 0xFFFF
}

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

//WriteToServer returns true if the FunctionCode is a write.
//FunctionCode 23 is both a read and write.
func (f FunctionCode) WriteToServer() bool {
	switch f {
	case 5, 6, 15, 16, 22, 23:
		return true
	}
	return false
}

//ReadToServer returns true if the FunctionCode is a write.
//FunctionCode 23 is both a read and write.
func (f FunctionCode) ReadToServer() bool {
	switch f {
	case 1, 2, 3, 4, 23:
		return true
	}
	return false
}

//WithoutError test if FunctionCode is an error response, and also return the version
//without error flag set
func (f FunctionCode) WithoutError() (bool, FunctionCode) {
	return f > 0x7f, f & 0x7f
}

//WithError return a copy of FunctionCode with the error flag set.
func (f FunctionCode) WithError() FunctionCode {
	return f + 0x80
}

//Modbus exception codes
type ExceptionCode byte

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

func (e ExceptionCode) Error() string {
	return fmt.Sprintf("ExceptionCode:0x%02X", byte(e))
}

func ToExceptionCode(err error) ExceptionCode {
	if err == nil {
		debugf("ToExceptionCode: unexpected covert nil error to ExceptionCode")
	}
	e, ok := err.(ExceptionCode)
	if ok {
		return e
	}
	if err == FcNotSupportedError {
		return EcIllegalFunction
	}
	return EcServerDeviceFailure
}

//Modbus Protocol Data Unit
type PDU []byte

//ExceptionReplyPacket make a PDU packet to reply to request req with ExceptionCode e
func ExceptionReplyPacket(req PDU, e ExceptionCode) PDU {
	fc := req.GetFunctionCode()
	return PDU([]byte{byte(fc), byte(e)})
}

//RepToWrite assumes the request is a write, and make the associated response
func (p PDU) MakeReply() PDU {
	if len(p) > 5 {
		return p[:5] //works for 5,6,15,16
	}
	return p
}

//MatchPDU returns true if ans is a valid reply to ask, including normal and
//error code replies.
func MatchPDU(ask PDU, ans PDU) bool {
	rf := ask.GetFunctionCode()
	af := ans.GetFunctionCode()
	return rf == af%128
}

//ValidateRequest tests for errors in a received Request PDU packet.
//Use ToExceptionCode to get the ExceptionCode for error
func (p PDU) ValidateRequest() error {
	if !p.GetFunctionCode().Valid() {
		return EcIllegalFunction
	}
	if len(p) < 3 {
		return EcIllegalDataAddress
	}
	//todo: check for errors 2 and 3
	return nil
}

//ValidateReply tests for errors and ExceptionCode in a received Reply PDU packet.
//And test other errors for p in context of request r.
//Use ToExceptionCode to get the ExceptionCode for error
func (p PDU) ValidateReply(r PDU) error {
	if len(p) < 2 {
		return fmt.Errorf("PDU too short")
	}
	ecflag, fc := p.GetFunctionCode().WithoutError()
	if fc != r.GetFunctionCode() {
		return fmt.Errorf("FunctionCode request %v != reply %v", r.GetFunctionCode(), fc)
	}
	if ecflag {
		return ExceptionCode(p[1])
	}

	return nil
}

//GetFunctionCode returns the funciton code
func (p PDU) GetFunctionCode() FunctionCode {
	if len(p) <= 0 {
		return FunctionCode(0)
	}
	return FunctionCode(p[0])
}

//GetAddress returns the stating address,
//If PDU is invalide, behavior is undefined (can panic).
func (p PDU) GetAddress() uint16 {
	return uint16(p[1])<<8 | uint16(p[2])
}

//GetRequestCount returns the number of values requested,
//If PDU is invalide, behavior is undefined (can panic).
func (p PDU) GetRequestCount() uint16 {
	max := p.GetFunctionCode().MaxPerPacket()
	if max < 2 {
		return max
	}
	return uint16(p[3])<<8 | uint16(p[4])
}

//GetRequestValues returns the values in a write request
func (p PDU) GetRequestValues() ([]byte, error) {
	max := p.GetFunctionCode().MaxPerPacket()
	if max == 0 {
		return nil, EcIllegalFunction
	}
	if max == 1 {
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
	if max <= 125 {
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

func (p PDU) MakeReplyData(data []byte) PDU {
	return PDU(append([]byte{byte(p.GetFunctionCode()), byte(len(data))}, data...))
}

func (p PDU) MakeRequestData(data []byte) PDU {
	fc := p.GetFunctionCode()
	switch fc {
	case FcWriteSingleCoil, FcWriteSingleRegister:
		return append(p[:3], data...)
	case FcWriteMultipleCoils, FcWriteMultipleRegisters:
		return append(p[:5], data...)
	}
	debugf("MakeRequestData unsupported for %v\n", fc)
	return nil
}
