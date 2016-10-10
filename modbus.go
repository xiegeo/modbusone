package modbusone

import (
	"fmt"
)

type ProtocalHandler interface {
	//OnInput is called on the server for a write request,
	//or on the client for read reply.
	OnInput(in PDU) error

	//OnOutput is called on the server for a read request,
	//or on the client before write requst.
	OnOutput(req PDU) (out PDU, err error)

	//OnError is called on the client when it receive a well formed
	//error from server
	OnError(req PDU, errRep PDU)
}

//Modebus function codes
type FunctionCode byte

const (
	FcReadCoils                  FunctionCode = 1
	FcReadDiscreteInputs         FunctionCode = 2
	FcReadHoldingRegisters       FunctionCode = 3
	FcReadInputRegisters         FunctionCode = 4
	FcWriteSingleCoil            FunctionCode = 5
	FcWriteSingleRegister        FunctionCode = 6
	FcWriteMultipleCoils         FunctionCode = 15
	FcWriteMultipleRegisters     FunctionCode = 16
	FcMaskWriteRegister          FunctionCode = 22
	FcReadWriteMultipleRegisters FunctionCode = 23
	//FcReadFIFOQueue              FunctionCode = 24 //not supported for now
)

//Valid test if FunctionCode is a allowed function, and not an error response
func (f FunctionCode) Valid() bool {
	return (f > 0 && f < 7) || (f > 14 && f < 17) || (f > 21 && f < 23)
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

//IsError test if FunctionCode is an error response, and also return the version
//without error flag set
func (f FunctionCode) IsError() (bool, FunctionCode) {
	return f > 0x0f, f & 0x0f
}

//WithError return a copy of FunctionCode with the error flag set.
func (f FunctionCode) WithError() (bool, FunctionCode) {
	return f > 0x0f, f & 0x0f
}

//Modbus exception codes
type ExceptionCode byte

const (
	//OK is invented for no error
	EcOK                                 ExceptionCode = 0
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
	return fmt.Sprintf("ExceptionCode:%v", e)
}

//Modbus Protocol Data Unit
type PDU []byte

//ErrorReplyPacket make a PDU packet to reply to request req with ExceptionCode e
func ErrorReplyPacket(req PDU, e ExceptionCode) PDU {
	fc := req.GetFunctionCode()
	return PDU([]byte{byte(fc), byte(e)})
}

//RepToWrite assumes the request is a write, and make the associated response
func (p PDU) RepToWrite() PDU{
	if len(p) > 5{
		p = p[:5] //works for 5,6,15,16
	}
	return p
}

//Validate tests for errors in a received PDU packet.
//Returns EcOK if packet is valid,
func (p PDU) Validate() ExceptionCode {
	if !p.GetFunctionCode().Valid() {
		return EcIllegalFunction
	}
	//todo: check for errors 2 and 3
	return EcOK
}

func (p PDU) GetFunctionCode() FunctionCode {
	if len(p) <= 0 {
		return FunctionCode(0)
	}
	return FunctionCode(p[0])
}
