package modbusone

import (
	"errors"
)

var FcNotSupportedError = errors.New("this FunctionCode is not supported")

type SimpleHandler struct {
	ReadDiscreteInputs  func(address, quantity uint16) ([]bool, error)
	WriteDiscreteInputs func(address uint16, values []bool) error

	ReadCoils  func(address, quantity uint16) ([]bool, error)
	WriteCoils func(address uint16, values []bool) error

	ReadInputRegisters  func(address, quantity uint16) ([]uint16, error)
	WriteInputRegisters func(address uint16, values []uint16) error

	ReadHoldingRegisters  func(address, quantity uint16) ([]uint16, error)
	WriteHoldingRegisters func(address uint16, values []uint16) error

	OnError func(req PDU, errRep PDU)
}

func (h *SimpleHandler) OnInput(in PDU) error {
	switch in.GetFunctionCode() {
	case FcReadDiscreteInputs:

	}
	return nil
}
