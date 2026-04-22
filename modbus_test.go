package modbusone_test

import (
	"errors"
	"fmt"
	"io"
	"math"
	"reflect"
	"testing"
	"time"

	. "github.com/xiegeo/modbusone"
)

func TestToExceptionCode(t *testing.T) {
	tests := []struct {
		err  error
		want ExceptionCode
	}{
		{err: EcIllegalDataAddress, want: EcIllegalDataAddress},
		{err: ErrFcNotSupported, want: EcIllegalFunction},
		{err: errors.New("other errors"), want: EcServerDeviceFailure},
	}
	for _, tt := range tests {
		t.Run(tt.err.Error(), func(t *testing.T) {
			if got := ToExceptionCode(tt.err); !reflect.DeepEqual(got, tt.want) {
				t.Errorf("ToExceptionCode() = %v, want %v", got, tt.want)
			}
			errWarped := fmt.Errorf("warped error for %w", tt.err)
			if got := ToExceptionCode(errWarped); !reflect.DeepEqual(got, tt.want) {
				t.Errorf("ToExceptionCode() = %v, want %v", got, tt.want)
			}
		})
	}
}

func generateClientServer(id byte) (*RTUClient, *RTUServer) {
	baudRate := int64(math.MaxInt64)

	// Open serial connections:
	clientSerial, serverSerial := newInternalSerial()

	clientSerialContext := NewSerialContext(clientSerial, baudRate)
	serverSerialContext := NewSerialContext(serverSerial, baudRate)

	client := NewRTUClient(clientSerialContext, id)
	server := NewRTUServer(serverSerialContext, id)

	return client, server
}

type noResponseHandler struct{ onErrorImp func(req PDU, errRep PDU) }

func (h *noResponseHandler) OnRead(req PDU) ([]byte, error) {
	return nil, ErrDoNotRespond
}

func (h *noResponseHandler) OnWrite(req PDU, data []byte) error {
	return ErrDoNotRespond
}

func (h *noResponseHandler) OnError(req PDU, errRep PDU) {
	h.onErrorImp(req, errRep)
}

type noOpHandler struct{ onErrorImp func(req PDU, errRep PDU) }

func (h *noOpHandler) OnRead(req PDU) ([]byte, error) {
	return req.MakeReadReply(req), nil
}

func (h *noOpHandler) OnWrite(req PDU, data []byte) error {
	return nil
}

func (h *noOpHandler) OnError(req PDU, errRep PDU) {
	h.onErrorImp(req, errRep)
}

func onErrorImpForTest(t *testing.T) func(req PDU, errRep PDU) {
	return func(req PDU, errRep PDU) {
		t.Errorf("received unexpected error:%x in request:%x", errRep, req)
	}
}

func TestErrDoNotRespond(t *testing.T) {
	resp := &noResponseHandler{}
	user := &noOpHandler{}
	id := byte(2)
	PDUs := []PDU{}
	for _, fc := range []FunctionCode{
		FcReadCoils, FcReadDiscreteInputs, FcReadHoldingRegisters, FcReadInputRegisters,
		/*FcWriteMultipleCoils, FcWriteMultipleRegisters, FcWriteSingleCoil,*/ FcWriteSingleRegister,
	} {
		var err error
		PDUs, err = MakePDURequestHeaders(fc, 0, 1, PDUs)
		if err != nil {
			t.Errorf("MakePDURequestHeaders() error = %v", err)
		}
	}
	t.Run("Server", func(t *testing.T) {
		termChan := make(chan error)
		client, server := generateClientServer(id)
		user.onErrorImp = onErrorImpForTest(t)
		go client.Serve(user)
		go func() {
			resp.onErrorImp = onErrorImpForTest(t)
			err := server.Serve(resp)
			if err != nil {
				if !errors.Is(err, io.ErrClosedPipe) {
					t.Errorf("server.Serve() error = %v", err)
				}
			}
			termChan <- err
		}()
		client.SetServerProcessingTime(100 * time.Millisecond) // faster timeouts during testing
		for _, p := range PDUs {
			t.Run(fmt.Sprint(p.GetFunctionCode()), func(t *testing.T) {
				err := client.DoTransaction(p)
				if !errors.Is(err, ErrServerTimeOut) {
					t.Errorf("client.DoTransaction() error = %v, want ErrServerTimeOut", err)
				}
			})
		}
		server.Close()
		<-termChan
	})
}
