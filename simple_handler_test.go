package modbusone_test

import (
	"errors"
	"os"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/xiegeo/modbusone"
	. "github.com/xiegeo/modbusone"
)

func FuzzOnWrite(f *testing.F) {
	//run: go test -fuzz FuzzHandler -fuzztime=60s

	SetDebugOut(os.Stdout)
	defer func() { SetDebugOut(nil) }()

	f.Fuzz(func(t *testing.T,
		req_ []byte, data []byte,
	) {
		req := PDU(req_)
		err := req.ValidateRequest()
		if err != nil {
			return
		}
		h0 := &modbusone.SimpleHandler{}
		err0 := h0.OnWrite(req, data)
		//t.Log("h0 error:", err0)
		hw := &modbusone.SimpleHandler{
			WriteDiscreteInputs:   func(address uint16, values []bool) error { return nil },
			WriteCoils:            func(address uint16, values []bool) error { return nil },
			WriteInputRegisters:   func(address uint16, values []uint16) error { return nil },
			WriteHoldingRegisters: func(address uint16, values []uint16) error { return nil },
		}
		err = hw.OnWrite(req, data)
		if err != nil {
			if errors.Is(err, err0) {
				_, rcErr := req.GetRequestCount()
				if rcErr != nil && errors.Is(err, rcErr) {
					return
				}
			}
			if errors.Is(err, ErrFcNotSupported) {
				assert.False(t, req.GetFunctionCode().Valid())
				assert.Zero(t, req.GetFunctionCode().MaxPerPacket())
				return
			}
			if errors.Is(err, EcIllegalDataValue) || errors.Is(err, EcIllegalDataAddress) {
				count, _ := req.GetRequestCount()
				if count == 0 {
					return
				}
				if req.GetFunctionCode().IsBool() {
					if (int(count)+7)/8 != len(data) {
						return
					}
				} else {
					if len(data)%2 == 1 {
						return
					}
					if len(data)/2 != int(count) {
						return
					}
				}
			}
			if req.GetFunctionCode() == FcWriteSingleCoil {
				return // only two possible acceptable values
			}
			t.Fatal(err)
		}
		we := errors.New("example application error")
		hwe := &modbusone.SimpleHandler{
			WriteDiscreteInputs:   func(address uint16, values []bool) error { return we },
			WriteCoils:            func(address uint16, values []bool) error { return we },
			WriteInputRegisters:   func(address uint16, values []uint16) error { return we },
			WriteHoldingRegisters: func(address uint16, values []uint16) error { return we },
		}
		err = hwe.OnWrite(req, data)
		assert.ErrorIs(t, err, we)
	})
}
