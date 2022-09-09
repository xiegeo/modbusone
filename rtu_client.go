package modbusone

import (
	"encoding/hex"
	"errors"
	"fmt"
	"sync/atomic"
	"time"
)

// RTUClient implements Client/Master side logic for RTU over a SerialContext to
// be used by a ProtocolHandler.
type RTUClient struct {
	com                  SerialContext
	packetReader         PacketReader
	SlaveID              byte
	serverProcessingTime time.Duration
	actions              chan rtuAction
}

// Client interface can both start and serve transactions.
type Client interface {
	ServerCloser
	RTUTransactionStarter
	DoTransaction(req PDU) error
}

// Asserts that RTUClient implements Client.
var _ Client = &RTUClient{}

// NewRTUClient create a new client communicating over SerialContext with the
// given slaveID as default.
func NewRTUClient(com SerialContext, slaveID byte) *RTUClient {
	pr, ok := com.(PacketReader)
	if !ok {
		pr = NewRTUPacketReader(com, true)
	}
	r := RTUClient{
		com:                  com,
		packetReader:         pr,
		SlaveID:              slaveID,
		serverProcessingTime: time.Second,
		actions:              make(chan rtuAction),
	}
	return &r
}

// SetServerProcessingTime sets the time to wait for a server response, the total
// wait time also includes the time needed for data transmission.
func (c *RTUClient) SetServerProcessingTime(t time.Duration) {
	c.serverProcessingTime = t
}

// GetTransactionTimeOut returns the total time to wait for a transaction
// (server response) to time out, given the expected length of RTU packets.
// This function is also used internally to calculate timeout.
func (c *RTUClient) GetTransactionTimeOut(reqLen, ansLen int) time.Duration {
	l := reqLen + ansLen
	return c.com.BytesDelay(l) + c.serverProcessingTime
}

type rtuAction struct {
	t       clientActionType
	data    RTU
	err     error
	errChan chan<- error
}

// ErrServerTimeOut is the time out error for StartTransaction.
var ErrServerTimeOut = errors.New("server timed out")

type clientActionType int

const (
	clientStart clientActionType = iota
	clientRead
	clientError
)

func (a clientActionType) String() string {
	switch a {
	case clientStart:
		return "start"
	case clientRead:
		return "read"
	case clientError:
		return "error"
	}
	return fmt.Sprintf("clientActionType %d", a)
}

// Serve serves RTUClient handlers.
func (c *RTUClient) Serve(handler ProtocolHandler) error {
	defer c.Close()
	go func() {
		// Reader loop that always ready to received data. This make sure that read
		// data is always new(ish), to dump data out that is received during an
		// unexpected time.
		for {
			rb := make([]byte, MaxRTUSize)
			n, err := c.packetReader.Read(rb)
			if err != nil {
				debugf("RTUClient read err:%v\n", err)
				c.actions <- rtuAction{t: clientError, err: err}
				c.Close()
				break
			}
			r := RTU(rb[:n])
			debugf("RTUClient read packet:%v\n", hex.EncodeToString(r))
			c.actions <- rtuAction{t: clientRead, data: r}
		}
	}()

	for {
		act := <-c.actions
		switch act.t {
		default:
			atomic.AddInt64(&c.com.Stats().OtherDrops, 1)
			debugf("RTUClient drop unexpected: %v", act)
			continue
		case clientError:
			return act.err
		case clientStart:
		}
		ap := act.data.fastGetPDU()
		afc := ap.GetFunctionCode()
		if afc.IsWriteToServer() {
			data, err := handler.OnRead(ap)
			if err != nil {
				act.errChan <- err
				continue
			}
			act.data = MakeRTU(act.data[0], ap.MakeWriteRequest(data))
			ap = act.data.fastGetPDU()
		}
		time.Sleep(c.com.MinDelay())
		_, err := c.com.Write(act.data)
		if err != nil {
			act.errChan <- err
			return err
		}
		if act.data[0] == 0 {
			time.Sleep(c.com.BytesDelay(len(act.data)))
			act.errChan <- nil // always success
			continue           // do not wait for read on multicast
		}

		timeOutChan := time.After(c.GetTransactionTimeOut(len(act.data), MaxRTUSize))

	READ_LOOP:
		for {
		SELECT:
			select {
			case <-timeOutChan:
				act.errChan <- ErrServerTimeOut
				break READ_LOOP
			case react := <-c.actions:
				switch react.t {
				default:
					err := fmt.Errorf("unexpected action:%s", react.t)
					act.errChan <- err
					return err
				case clientError:
					return react.err
				case clientRead:
					// test for read error
					if react.err != nil {
						return react.err
					}
				}
				if react.data[0] != act.data[0] {
					atomic.AddInt64(&c.com.Stats().IDDrops, 1)
					debugf("RTUClient unexpected slaveId:%v in %v\n", act.data[0], hex.EncodeToString(react.data))
					break SELECT
				}
				rp, err := react.data.GetPDU()
				if err != nil {
					if errors.Is(err, ErrorCrc) {
						atomic.AddInt64(&c.com.Stats().CrcErrors, 1)
					} else {
						atomic.AddInt64(&c.com.Stats().OtherErrors, 1)
					}
					act.errChan <- err
					break READ_LOOP
				}
				hasErr, fc := rp.GetFunctionCode().SeparateError()
				if hasErr && fc == afc {
					atomic.AddInt64(&c.com.Stats().OtherErrors, 1)
					handler.OnError(ap, rp)
					act.errChan <- fmt.Errorf("server reply with exception:%v", hex.EncodeToString(rp))
					break READ_LOOP
				}
				if !IsRequestReply(act.data.fastGetPDU(), rp) {
					atomic.AddInt64(&c.com.Stats().OtherErrors, 1)
					act.errChan <- fmt.Errorf("unexpected reply:%v", hex.EncodeToString(rp))
					break READ_LOOP
				}
				if afc.IsReadToServer() {
					// read from server, write here
					bs, err := rp.GetReplyValues()
					if err != nil {
						atomic.AddInt64(&c.com.Stats().OtherErrors, 1)
						act.errChan <- err
						break READ_LOOP
					}
					err = handler.OnWrite(ap, bs)
					if err != nil {
						atomic.AddInt64(&c.com.Stats().OtherErrors, 1)
					}
					act.errChan <- err // success if nil
					break READ_LOOP
				}
				act.errChan <- nil // success
				break READ_LOOP
			}
		}
	}
}

// Close closes the client and closes the connection.
func (c *RTUClient) Close() error {
	return c.com.Close()
}

// DoTransaction starts a transaction, and returns a channel that returns an error
// or nil, with the default slaveID.
//
// DoTransaction is blocking.
//
// For read from server, the PDU is sent as is (after been warped up in RTU)
// For write to server, the data part given will be ignored, and filled in by data from handler.
func (c *RTUClient) DoTransaction(req PDU) error {
	errChan := make(chan error)
	c.StartTransactionToServer(c.SlaveID, req, errChan)
	return <-errChan
}

// StartTransactionToServer starts a transaction, with a custom slaveID.
// errChan is required, an error is set is the transaction failed, or
// nil for success.
//
// StartTransactionToServer is not blocking.
//
// For read from server, the PDU is sent as is (after been warped up in RTU)
// For write to server, the data part given will be ignored, and filled in by data from handler.
func (c *RTUClient) StartTransactionToServer(slaveID byte, req PDU, errChan chan error) {
	c.actions <- rtuAction{t: clientStart, data: MakeRTU(slaveID, req), errChan: errChan}
}

// RTUTransactionStarter is an interface implemented by RTUClient.
type RTUTransactionStarter interface {
	StartTransactionToServer(slaveID byte, req PDU, errChan chan error)
}

// DoTransactions runs the reqs transactions in order.
// If any error is encountered, it returns early and reports the index number and
// error message.
func DoTransactions(c RTUTransactionStarter, slaveID byte, reqs []PDU) (int, error) {
	errChan := make(chan error)
	for i, r := range reqs {
		c.StartTransactionToServer(slaveID, r, errChan)
		err := <-errChan
		if err != nil {
			return i, err
		}
	}
	return len(reqs), nil
}

// MakePDURequestHeaders generates the list of PDU request headers by splitting quantity
// into allowed sizes.
// Returns an error if quantity is out of range.
func MakePDURequestHeaders(fc FunctionCode, address, quantity uint16, appendTO []PDU) ([]PDU, error) {
	return MakePDURequestHeadersSized(fc, address, quantity, fc.MaxPerPacket(), appendTO)
}

// MakePDURequestHeadersSized generates the list of PDU request headers by splitting quantity
// into sizes of maxPerPacket or less.
// Returns an error if quantity is out of range.
//
// You can use FunctionCode.MaxPerPacketSized to calculate one with the wanted byte length.
func MakePDURequestHeadersSized(fc FunctionCode, address, quantity uint16, maxPerPacket uint16, appendTO []PDU) ([]PDU, error) {
	if uint(address)+uint(quantity) > uint(fc.MaxRange()) {
		return nil, fmt.Errorf("quantity is out of range")
	}
	q := maxPerPacket
	for quantity > 0 {
		if quantity < maxPerPacket {
			q = quantity
		}
		pdu, err := fc.MakeRequestHeader(address, q)
		if err != nil {
			return nil, err
		}
		appendTO = append(appendTO, pdu)
		address += q
		quantity -= q
	}
	return appendTO, nil
}
