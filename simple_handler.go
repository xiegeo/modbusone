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

func (h *SimpleHandler) OnWrite(req PDU, data []byte) error {
	fc := req.GetFunctionCode()
	address := req.GetAddress()
	count := req.GetRequestCount()
	var err error
	switch fc {
	case FcReadDiscreteInputs:
		if h.WriteDiscreteInputs == nil {
			return EcIllegalFunction
		}
		values, err := DataToBools(data, req.GetRequestCount(), fc)
		if err != nil {
			return err
		}
		return h.WriteDiscreteInputs(req.GetAddress(), values)
	case FcReadCoils, FcWriteSingleCoil, FcWriteMultipleCoils:
		if h.WriteCoils == nil {
			return EcIllegalFunction
		}
		values, err := DataToBools(data, req.GetRequestCount(), fc)
		if err != nil {
			return err
		}
		return h.WriteDiscreteInputs(req.GetAddress(), values)
	}
	return nil
}
