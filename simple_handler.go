package modbusone

import (
	"errors"
)

//ErrFcNotSupported is another version of EcIllegalFunction, encountering of
//this error shows the error is locally generated, not a remote ExceptionCode.
var ErrFcNotSupported = errors.New("this FunctionCode is not supported")

var _ ProtocolHandler = &SimpleHandler{} //assert implents

//SimpleHandler implements ProtocolHandler, any nil function returns ErrFcNotSupported
type SimpleHandler struct {

	//ReadDiscreteInputs handles server side FC=2
	ReadDiscreteInputs func(address, quantity uint16) ([]bool, error)
	//ReadDiscreteInputs handles client side FC=2
	WriteDiscreteInputs func(address uint16, values []bool) error

	//ReadCoils handles client side FC=5&15, server side FC=1
	ReadCoils func(address, quantity uint16) ([]bool, error)
	//WriteCoils handles client side FC=1, server side FC=5&15
	WriteCoils func(address uint16, values []bool) error

	//ReadInputRegisters handles server side FC=4
	ReadInputRegisters func(address, quantity uint16) ([]uint16, error)
	//ReadDiscreteInputs handles client side FC=4
	WriteInputRegisters func(address uint16, values []uint16) error

	//ReadHoldingRegisters handles client side FC=6&16, server side FC=3
	ReadHoldingRegisters func(address, quantity uint16) ([]uint16, error)
	//WriteHoldingRegisters handles client side FC=3, server side FC=6&16
	WriteHoldingRegisters func(address uint16, values []uint16) error

	//OnErrorImp handles OnError
	OnErrorImp func(req PDU, errRep PDU)
}

//OnRead is called by a Server, set Read... to catch the calls.
func (h *SimpleHandler) OnRead(req PDU) ([]byte, error) {
	fc := req.GetFunctionCode()
	address := req.GetAddress()
	count, err := req.GetRequestCount()
	if err != nil {
		return nil, err
	}

	switch fc {
	case FcReadDiscreteInputs:
		if h.ReadDiscreteInputs == nil {
			return nil, ErrFcNotSupported
		}
		values, err := h.ReadDiscreteInputs(address, count)
		if err != nil {
			return nil, err
		}
		return BoolsToData(values, fc)
	case FcReadCoils, FcWriteSingleCoil, FcWriteMultipleCoils:
		if h.ReadCoils == nil {
			return nil, ErrFcNotSupported
		}
		values, err := h.ReadCoils(address, count)
		if err != nil {
			return nil, err
		}
		return BoolsToData(values, fc)
	case FcReadInputRegisters:
		if h.ReadInputRegisters == nil {
			return nil, ErrFcNotSupported
		}
		values, err := h.ReadInputRegisters(address, count)
		if err != nil {
			return nil, err
		}
		return RegistersToData(values)
	case FcReadHoldingRegisters, FcWriteSingleRegister, FcWriteMultipleRegisters:
		if h.ReadHoldingRegisters == nil {
			return nil, ErrFcNotSupported
		}
		values, err := h.ReadHoldingRegisters(address, count)
		if err != nil {
			return nil, err
		}
		return RegistersToData(values)
	}
	return nil, ErrFcNotSupported
}

//OnWrite is called by a Server, set Write... to catch the calls.
func (h *SimpleHandler) OnWrite(req PDU, data []byte) error {
	fc := req.GetFunctionCode()
	address := req.GetAddress()
	count, err := req.GetRequestCount()
	if err != nil {
		return err
	}
	switch fc {
	case FcReadDiscreteInputs:
		if h.WriteDiscreteInputs == nil {
			return ErrFcNotSupported
		}
		values, err := DataToBools(data, count, fc)
		if err != nil {
			return err
		}
		return h.WriteDiscreteInputs(address, values)
	case FcReadCoils, FcWriteSingleCoil, FcWriteMultipleCoils:
		if h.WriteCoils == nil {
			return ErrFcNotSupported
		}
		values, err := DataToBools(data, count, fc)
		if err != nil {
			return err
		}
		return h.WriteCoils(address, values)
	case FcReadInputRegisters:
		if h.WriteInputRegisters == nil {
			return ErrFcNotSupported
		}
		values, err := DataToRegisters(data)
		if err != nil {
			return err
		}
		return h.WriteInputRegisters(address, values)
	case FcReadHoldingRegisters, FcWriteSingleRegister, FcWriteMultipleRegisters:
		if h.WriteHoldingRegisters == nil {
			return ErrFcNotSupported
		}
		values, err := DataToRegisters(data)
		if err != nil {
			return err
		}
		return h.WriteHoldingRegisters(address, values)
	}
	return ErrFcNotSupported
}

//OnError is called by a Server, set OnErrorImp to catch the calls
func (h *SimpleHandler) OnError(req PDU, errRep PDU) {
	if h.OnErrorImp == nil {
		return
	}
	h.OnErrorImp(req, errRep)
}
