package modbusone

import (
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"time"
)

//RTUClient implements Client/Master side logic for RTU over a SerialContext to
//be used by a ProtocalHandler
type RTUClient struct {
	com                  SerialContext
	packetReader         io.Reader
	SlaveID              byte
	serverProcessingTime time.Duration
	actions              chan rtuAction
}

//RTUClient is a Server
var _ Server = &RTUClient{}

//NewRTUCLient create a new client communicating over SerialContext with the
//give slaveID as default.
func NewRTUCLient(com SerialContext, slaveID byte) *RTUClient {
	r := RTUClient{
		com:                  com,
		packetReader:         NewRTUPacketReader(com, true, StartingSerialBufferSide),
		SlaveID:              slaveID,
		serverProcessingTime: time.Second,
		actions:              make(chan rtuAction),
	}
	return &r
}

//SetServerProcessingTime sets the time to wait for a server response, the total
//wait time also includes the time needed for data transmission
func (c *RTUClient) SetServerProcessingTime(t time.Duration) {
	c.serverProcessingTime = t
}

//GetTransactionTimeOut returns the total time to wait for a transaction
//(server response) to time out, given the expected length of RTU packets.
//This function is also used internally to calculate timeout.
func (c *RTUClient) GetTransactionTimeOut(reqLen, ansLen int) time.Duration {
	l := reqLen + ansLen
	return c.com.BytesDelay(l) + c.serverProcessingTime
}

type rtuAction struct {
	t       actionType
	data    RTU
	errChan chan<- error
}

//ErrServerTimeOut is the time out error for StartTransaction
var ErrServerTimeOut = errors.New("server timed out")

type actionType int

const (
	start actionType = 1
	read  actionType = 2
)

func (a actionType) String() string {
	switch a {
	case start:
		return "start"
	case read:
		return "read"
	}
	return fmt.Sprintf("actionType %d", a)
}

//Serve serves RTUClient side handlers, must close SerialContext after error is
//returned, to clean up.
func (c *RTUClient) Serve(handler ProtocalHandler) error {
	delay := c.com.MinDelay()

	var ioerr error //irrecoverable io errors
	var readerr error
	go func() {
		//Reader loop that always ready to received data. This make sure that read
		//data is always new(ish), to dump data out that is received during an
		//unexpected time.
		rb := make([]byte, MaxRTUSize)
		for {
			n, err := c.packetReader.Read(rb)
			if err != nil {
				readerr = err
				debugf("RTUClient read err:%v\n", err)
			}
			r := RTU(rb[:n])
			debugf("RTUClient read packet:%v\n", hex.EncodeToString(r))
			c.actions <- rtuAction{read, r, nil}
		}
	}()

	hasError := func() bool {
		return ioerr != nil || readerr != nil
	}
	getError := func() error {
		if ioerr != nil {
			return ioerr
		}
		return readerr
	}
	sendError := func(ec chan<- error, err error) error {
		if ec != nil {
			ec <- err
		}
		return err
	}
	sendGetError := func(ec chan<- error) error {
		return sendError(ec, getError())
	}

	for {
		act, ok := <-c.actions
		if !ok {
			debugf("RTUClient actions closed\n")
			return getError()
		}
		if act.t != start {
			debugf("RTUClient drop unexpected action:%s\n", act.t)
			continue
		}
		ap := act.data.fastGetPDU()
		afc := ap.GetFunctionCode()
		if afc.IsWriteToServer() {
			data, err := handler.OnRead(ap)
			if err != nil {
				sendError(act.errChan, err)
				continue
			}
			act.data = MakeRTU(act.data[0], ap.MakeWriteRequest(data))
			ap = act.data.fastGetPDU()
		}
		time.Sleep(delay)
		_, ioerr = c.com.Write(act.data)
		if hasError() {
			return sendGetError(act.errChan)
		}
		if act.data[0] == 0 {
			continue // do not wait for read on multicast
		}
		timeOutChan := time.After(c.GetTransactionTimeOut(len(act.data), MaxRTUSize))

	READ_LOOP:
		for {
		SELECT:
			select {
			case <-timeOutChan:
				sendError(act.errChan, ErrServerTimeOut)
				break READ_LOOP
			case react, ok := <-c.actions:
				if !ok {
					debugf("RTUClient actions closed\n")
					return sendGetError(act.errChan)
				}
				if react.t != read {
					ioerr = fmt.Errorf("unexpected action:%s", react.t)
					return sendGetError(act.errChan)
				}
				if react.data[0] != act.data[0] {
					debugf("RTUClient unexpected slaveId:%v in %v\n", act.data[0], hex.EncodeToString(react.data))
					break SELECT
				}
				rp, err := react.data.GetPDU()
				if err != nil {
					sendError(act.errChan, err)
					break READ_LOOP
				}
				hasErr, fc := rp.GetFunctionCode().SeparateError()
				if hasErr && fc == afc {
					handler.OnError(ap, rp)
					sendError(act.errChan, fmt.Errorf("server reply with exception:%v", hex.EncodeToString(rp)))
					break READ_LOOP
				}
				if !MatchPDU(act.data.fastGetPDU(), rp) {
					sendError(act.errChan, fmt.Errorf("unexpected reply:%v", hex.EncodeToString(rp)))
					break READ_LOOP
				}
				if !afc.IsWriteToServer() {
					//read from server, write here
					bs, err := rp.GetReplyValues()
					if err != nil {
						sendError(act.errChan, err)
						break READ_LOOP
					}
					err = handler.OnWrite(ap, bs)
					sendError(act.errChan, err)
					break READ_LOOP
				}
				sendError(act.errChan, nil)
				break READ_LOOP
			}
		}
	}
}

//DoTransaction starts a transaction, and returns a channel that returns an error
//or nil, with the default slaveID.
//
//DoTransaction is blocking.
//
//For read from server, the PDU is sent as is (after been warped up in RTU)
//For write to server, the data part given will be ignored, and filled in by data from handler.
func (c *RTUClient) DoTransaction(req PDU) error {
	errChan := make(chan error)
	c.StartTransactionToServer(c.SlaveID, req, errChan)
	return <-errChan
}

//StartTransactionToServer starts a transaction, with a custom slaveID.
//errChan is required and usable, an error is set is the transaction failed, or
//nil for success.
//
//StartTransactionToServer is not blocking.
//
//For read from server, the PDU is sent as is (after been warped up in RTU)
//For write to server, the data part given will be ignored, and filled in by data from handler.
func (c *RTUClient) StartTransactionToServer(slaveID byte, req PDU, errChan chan error) {
	c.actions <- rtuAction{start, MakeRTU(slaveID, req), errChan}
}

//RTUTransactionStarter is an interface implemented by RTUClient.
type RTUTransactionStarter interface {
	StartTransactionToServer(slaveID byte, req PDU, errChan chan error)
}

//DoTransactions runs the reqs transactions in order.
//If any error is encountered, it returns early and reports the index number and
//error message
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

//MakePDURequestHeaders generates the list of PDU request headers by spliting quantity
//into allowed sizes.
//Returns an error if quantitiy is out of range.
func MakePDURequestHeaders(fc FunctionCode, address uint16, quantity uint16, appendTO []PDU) ([]PDU, error) {
	if uint(address)+uint(quantity) > uint(fc.MaxRange()) {
		return nil, fmt.Errorf("quantitiy is out of range")
	}
	r := fc.MaxPerPacket()
	q := r
	for quantity > 0 {
		if quantity < r {
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
