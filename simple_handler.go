package modbusone

import (
	"errors"
)

var FcNotSupportedError = errors.New("this FunctionCode is not supported")

var _ ProtocalHandler = &SimpleHandler{}

//SimpleHandler implements ProtocalHandler
type SimpleHandler struct {
	ReadDiscreteInputs  func(address, quantity uint16) ([]bool, error)
	WriteDiscreteInputs func(address uint16, values []bool) error

	ReadCoils  func(address, quantity uint16) ([]bool, error)
	WriteCoils func(address uint16, values []bool) error

	ReadInputRegisters  func(address, quantity uint16) ([]uint16, error)
	WriteInputRegisters func(address uint16, values []uint16) error

	ReadHoldingRegisters  func(address, quantity uint16) ([]uint16, error)
	WriteHoldingRegisters func(address uint16, values []uint16) error

	OnErrorImp func(req PDU, errRep PDU)
}

func (h *SimpleHandler) OnRead(req PDU) ([]byte, error) {
	fc := req.GetFunctionCode()
	address := req.GetAddress()
	count := req.GetRequestCount()

	switch fc {
	case FcReadDiscreteInputs:
		if h.ReadDiscreteInputs == nil {
			return nil, FcNotSupportedError
		}
		values, err := h.ReadDiscreteInputs(address, count)
		if err != nil {
			return nil, err
		}
		return BoolsToData(values, fc)
	case FcReadCoils, FcWriteSingleCoil, FcWriteMultipleCoils:
		if h.ReadCoils == nil {
			return nil, FcNotSupportedError
		}
		values, err := h.ReadCoils(address, count)
		if err != nil {
			return nil, err
		}
		return BoolsToData(values, fc)
	case FcReadInputRegisters:
		if h.ReadInputRegisters == nil {
			return nil, FcNotSupportedError
		}
		values, err := h.ReadInputRegisters(address, count)
		if err != nil {
			return nil, err
		}
		return RegistersToData(values)
	case FcReadHoldingRegisters, FcWriteSingleRegister, FcWriteMultipleRegisters:
		if h.ReadHoldingRegisters == nil {
			return nil, FcNotSupportedError
		}
		values, err := h.ReadHoldingRegisters(address, count)
		if err != nil {
			return nil, err
		}
		return RegistersToData(values)
	}
	return nil, FcNotSupportedError
}

func (h *SimpleHandler) OnWrite(req PDU, data []byte) error {
	fc := req.GetFunctionCode()
	address := req.GetAddress()
	count := req.GetRequestCount()
	switch fc {
	case FcReadDiscreteInputs:
		if h.WriteDiscreteInputs == nil {
			return FcNotSupportedError
		}
		values, err := DataToBools(data, count, fc)
		if err != nil {
			return err
		}
		return h.WriteDiscreteInputs(address, values)
	case FcReadCoils, FcWriteSingleCoil, FcWriteMultipleCoils:
		if h.WriteCoils == nil {
			return FcNotSupportedError
		}
		values, err := DataToBools(data, count, fc)
		if err != nil {
			return err
		}
		return h.WriteCoils(address, values)
	case FcReadInputRegisters:
		if h.WriteInputRegisters == nil {
			return FcNotSupportedError
		}
		values, err := DataToRegisters(data)
		if err != nil {
			return err
		}
		return h.WriteInputRegisters(address, values)
	case FcReadHoldingRegisters, FcWriteSingleRegister, FcWriteMultipleRegisters:
		if h.WriteHoldingRegisters == nil {
			return FcNotSupportedError
		}
		values, err := DataToRegisters(data)
		if err != nil {
			return err
		}
		return h.WriteHoldingRegisters(address, values)
	}
	return FcNotSupportedError
}

func (h *SimpleHandler) OnError(req PDU, errRep PDU) {
	if h.OnErrorImp == nil {
		return
	}
	h.OnErrorImp(req, errRep)
}
