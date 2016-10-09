package modbusone

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
	FcReadFIFOQueue              FunctionCode = 24
)

func (f FunctionCode) Valid() bool {
	return (f > 0 && f < 7) || (f > 14 && f < 17) || (f > 21 && f < 24)
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

//Modbus Protocol Data Unit
type ADU []byte

//Validate tests for errors in a received ADU packet.
//Returns EcOK if packet is valid,
//only use the Get functions after Validate passes
func (a ADU) Validate() ExceptionCode {
	la := len(a)
	if la < 0 || !a.GetFunctionCode().Valid() {
		return EcIllegalFunction
	}
	//todo: check for errors 2 and 3
	return EcOK
}

func (a ADU) GetFunctionCode() FunctionCode {
	return FunctionCode(a[0])
}
