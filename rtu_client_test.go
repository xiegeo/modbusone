package modbusone

import (
	"errors"
	"testing"
)

// mockProtocolHandler is a test double for ProtocolHandler that records calls.
type mockProtocolHandler struct {
	onReadFunc  func(req PDU) ([]byte, error)
	onWriteFunc func(req PDU, data []byte) error
	onErrorFunc func(req PDU, errRep PDU)

	onReadCalled  bool
	onWriteCalled bool
	onErrorCalled bool

	lastOnReadReq   PDU
	lastOnWriteReq  PDU
	lastOnWriteData []byte
	lastOnErrorReq  PDU
	lastOnErrorRep  PDU
}

func (m *mockProtocolHandler) OnRead(req PDU) ([]byte, error) {
	m.onReadCalled = true
	m.lastOnReadReq = req
	if m.onReadFunc != nil {
		return m.onReadFunc(req)
	}
	return nil, nil
}

func (m *mockProtocolHandler) OnWrite(req PDU, data []byte) error {
	m.onWriteCalled = true
	m.lastOnWriteReq = req
	m.lastOnWriteData = data
	if m.onWriteFunc != nil {
		return m.onWriteFunc(req, data)
	}
	return nil
}

func (m *mockProtocolHandler) OnError(req PDU, errRep PDU) {
	m.onErrorCalled = true
	m.lastOnErrorReq = req
	m.lastOnErrorRep = errRep
}

// makeTestRTU creates a valid RTU for testing with the given slaveID and a
// simple read-holding-registers PDU.
func makeTestRTU(slaveID byte) RTU {
	pdu, err := FcReadHoldingRegisters.MakeRequestHeader(0, 1)
	if err != nil {
		panic(err)
	}
	return MakeRTU(slaveID, pdu)
}

// makeWriteTestRTU creates a valid RTU for testing with a write-single-register PDU.
func makeWriteTestRTU(slaveID byte) RTU {
	pdu, err := FcWriteSingleRegister.MakeRequestHeader(0, 1)
	if err != nil {
		panic(err)
	}
	return MakeRTU(slaveID, pdu)
}

func TestUpgradeHandlerOnRead_MatchingSlaveID(t *testing.T) {
	const slaveID = byte(0x11)
	rtu := makeTestRTU(slaveID)
	expectedData := []byte{0x01, 0x02}

	mock := &mockProtocolHandler{
		onReadFunc: func(req PDU) ([]byte, error) {
			return expectedData, nil
		},
	}
	h := &upgradeHandler{slaveID: slaveID, handler: mock}

	data, err := h.OnRead(rtu)
	if err != nil {
		t.Fatalf("OnRead() unexpected error: %v", err)
	}
	if !mock.onReadCalled {
		t.Error("OnRead() did not call underlying handler.OnRead")
	}
	if len(data) != len(expectedData) {
		t.Errorf("OnRead() returned %v bytes, want %v", len(data), len(expectedData))
	}
	for i, b := range data {
		if b != expectedData[i] {
			t.Errorf("OnRead() data[%d] = %v, want %v", i, b, expectedData[i])
		}
	}
}

func TestUpgradeHandlerOnRead_MismatchedSlaveID(t *testing.T) {
	const rtuSlaveID = byte(0x11)
	const handlerSlaveID = byte(0x22)
	rtu := makeTestRTU(rtuSlaveID)

	mock := &mockProtocolHandler{}
	h := &upgradeHandler{slaveID: handlerSlaveID, handler: mock}

	_, err := h.OnRead(rtu)
	if err == nil {
		t.Fatal("OnRead() expected error for slaveID mismatch, got nil")
	}
	if mock.onReadCalled {
		t.Error("OnRead() should not call underlying handler on slaveID mismatch")
	}
}

func TestUpgradeHandlerOnRead_PropagatesHandlerError(t *testing.T) {
	const slaveID = byte(0x01)
	rtu := makeTestRTU(slaveID)
	expectedErr := errors.New("handler read error")

	mock := &mockProtocolHandler{
		onReadFunc: func(req PDU) ([]byte, error) {
			return nil, expectedErr
		},
	}
	h := &upgradeHandler{slaveID: slaveID, handler: mock}

	_, err := h.OnRead(rtu)
	if !errors.Is(err, expectedErr) {
		t.Errorf("OnRead() error = %v, want %v", err, expectedErr)
	}
}

func TestUpgradeHandlerOnRead_PassesPDUToHandler(t *testing.T) {
	const slaveID = byte(0x05)
	rtu := makeTestRTU(slaveID)
	expectedPDU := rtu.fastGetPDU()

	var capturedPDU PDU
	mock := &mockProtocolHandler{
		onReadFunc: func(req PDU) ([]byte, error) {
			capturedPDU = req
			return nil, nil
		},
	}
	h := &upgradeHandler{slaveID: slaveID, handler: mock}

	_, err := h.OnRead(rtu)
	if err != nil {
		t.Fatalf("OnRead() unexpected error: %v", err)
	}
	if string(capturedPDU) != string(expectedPDU) {
		t.Errorf("OnRead() passed PDU %v, want %v", capturedPDU, expectedPDU)
	}
}

func TestUpgradeHandlerOnWrite_MatchingSlaveID(t *testing.T) {
	const slaveID = byte(0x11)
	rtu := makeWriteTestRTU(slaveID)
	writeData := []byte{0x00, 0x0A}

	mock := &mockProtocolHandler{}
	h := &upgradeHandler{slaveID: slaveID, handler: mock}

	err := h.OnWrite(rtu, writeData)
	if err != nil {
		t.Fatalf("OnWrite() unexpected error: %v", err)
	}
	if !mock.onWriteCalled {
		t.Error("OnWrite() did not call underlying handler.OnWrite")
	}
}

func TestUpgradeHandlerOnWrite_MismatchedSlaveID(t *testing.T) {
	const rtuSlaveID = byte(0x11)
	const handlerSlaveID = byte(0x33)
	rtu := makeWriteTestRTU(rtuSlaveID)

	mock := &mockProtocolHandler{}
	h := &upgradeHandler{slaveID: handlerSlaveID, handler: mock}

	err := h.OnWrite(rtu, []byte{0x00, 0x0A})
	if err == nil {
		t.Fatal("OnWrite() expected error for slaveID mismatch, got nil")
	}
	if mock.onWriteCalled {
		t.Error("OnWrite() should not call underlying handler on slaveID mismatch")
	}
}

func TestUpgradeHandlerOnWrite_PropagatesHandlerError(t *testing.T) {
	const slaveID = byte(0x01)
	rtu := makeWriteTestRTU(slaveID)
	expectedErr := errors.New("handler write error")

	mock := &mockProtocolHandler{
		onWriteFunc: func(req PDU, data []byte) error {
			return expectedErr
		},
	}
	h := &upgradeHandler{slaveID: slaveID, handler: mock}

	err := h.OnWrite(rtu, []byte{0x00, 0x0A})
	if !errors.Is(err, expectedErr) {
		t.Errorf("OnWrite() error = %v, want %v", err, expectedErr)
	}
}

func TestUpgradeHandlerOnWrite_PassesPDUAndDataToHandler(t *testing.T) {
	const slaveID = byte(0x07)
	rtu := makeWriteTestRTU(slaveID)
	expectedPDU := rtu.fastGetPDU()
	writeData := []byte{0xAB, 0xCD}

	var capturedPDU PDU
	var capturedData []byte
	mock := &mockProtocolHandler{
		onWriteFunc: func(req PDU, data []byte) error {
			capturedPDU = req
			capturedData = data
			return nil
		},
	}
	h := &upgradeHandler{slaveID: slaveID, handler: mock}

	err := h.OnWrite(rtu, writeData)
	if err != nil {
		t.Fatalf("OnWrite() unexpected error: %v", err)
	}
	if string(capturedPDU) != string(expectedPDU) {
		t.Errorf("OnWrite() passed PDU %v, want %v", capturedPDU, expectedPDU)
	}
	if string(capturedData) != string(writeData) {
		t.Errorf("OnWrite() passed data %v, want %v", capturedData, writeData)
	}
}

func TestUpgradeHandlerOnError_DelegatesToHandler(t *testing.T) {
	const slaveID = byte(0x11)
	reqRTU := makeTestRTU(slaveID)
	// Build an error-reply PDU: function code with high bit set + exception code
	errPDU := PDU([]byte{byte(FcReadHoldingRegisters) | 0x80, byte(EcIllegalDataAddress)})
	errRTU := MakeRTU(slaveID, errPDU)

	mock := &mockProtocolHandler{}
	h := &upgradeHandler{slaveID: slaveID, handler: mock}

	h.OnError(reqRTU, errRTU)

	if !mock.onErrorCalled {
		t.Error("OnError() did not call underlying handler.OnError")
	}
	expectedReqPDU := reqRTU.fastGetPDU()
	expectedErrPDU := errRTU.fastGetPDU()
	if string(mock.lastOnErrorReq) != string(expectedReqPDU) {
		t.Errorf("OnError() passed req PDU %v, want %v", mock.lastOnErrorReq, expectedReqPDU)
	}
	if string(mock.lastOnErrorRep) != string(expectedErrPDU) {
		t.Errorf("OnError() passed errRep PDU %v, want %v", mock.lastOnErrorRep, expectedErrPDU)
	}
}

func TestUpgradeHandlerOnError_WithEmptyRTUs(t *testing.T) {
	// OnError should not panic even with unusual RTU inputs
	// An empty RTU would panic on fastGetPDU, so use minimal valid RTUs
	const slaveID = byte(0x01)
	reqRTU := makeTestRTU(slaveID)
	errPDU := PDU([]byte{byte(FcReadHoldingRegisters) | 0x80, byte(EcServerDeviceFailure)})
	errRTU := MakeRTU(slaveID, errPDU)

	mock := &mockProtocolHandler{}
	h := &upgradeHandler{slaveID: slaveID, handler: mock}

	// Should not panic
	h.OnError(reqRTU, errRTU)
	if !mock.onErrorCalled {
		t.Error("OnError() did not call underlying handler.OnError")
	}
}

// TestUpgradeHandlerImplementsRTUProtocolHandler verifies the interface compliance at compile time.
var _ RTUProtocolHandler = (*upgradeHandler)(nil)

func TestUpgradeHandlerSlaveIDMismatchMessage(t *testing.T) {
	// Verify the error message contains slaveID values for debugging
	const rtuSlaveID = byte(0x11)
	const handlerSlaveID = byte(0x22)
	rtu := makeTestRTU(rtuSlaveID)

	mock := &mockProtocolHandler{}
	h := &upgradeHandler{slaveID: handlerSlaveID, handler: mock}

	_, readErr := h.OnRead(rtu)
	if readErr == nil {
		t.Fatal("expected error from OnRead slaveID mismatch")
	}
	writeErr := h.OnWrite(rtu, []byte{0x00})
	if writeErr == nil {
		t.Fatal("expected error from OnWrite slaveID mismatch")
	}

	// Both errors should reference the SlaveID values to aid debugging
	readMsg := readErr.Error()
	writeMsg := writeErr.Error()
	if readMsg == "" {
		t.Error("OnRead mismatch error message is empty")
	}
	if writeMsg == "" {
		t.Error("OnWrite mismatch error message is empty")
	}
}