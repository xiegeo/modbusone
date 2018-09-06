package modbusone

import (
	"bytes"
	"encoding/hex"
	"fmt"
	"sync/atomic"
	"time"
)

//FailoverRTUClient implements Client/Master side logic for RTU over a SerialContext to
//be used by a ProtocolHandler with failover function.
type FailoverRTUClient struct {
	com                  *FailoverSerialConn
	packetReader         PacketReader
	SlaveID              byte
	serverProcessingTime time.Duration
	actions              chan rtuAction
}

//FailoverRTUClient is also a Server
var _ Server = &FailoverRTUClient{}

//NewFailoverRTUClient create a new client with failover function communicating over SerialContext with the
//give slaveID as default.
//
//If isFailover is true, it is the secondary.
func NewFailoverRTUClient(com SerialContext, isFailover bool, slaveID byte) *FailoverRTUClient {
	pr, ok := com.(*FailoverSerialConn)
	if !ok {
		pr = NewFailoverConn(com, isFailover, true)
	}
	if pr.isFailover != isFailover {
		panic("A SerialContext was provided with conflicting settings.")
	}
	r := FailoverRTUClient{
		com:                  pr,
		packetReader:         pr,
		SlaveID:              slaveID,
		serverProcessingTime: time.Second,
		actions:              make(chan rtuAction),
	}
	return &r
}

//SetServerProcessingTime sets the time to wait for a server response, the total
//wait time also includes the time needed for data transmission
func (c *FailoverRTUClient) SetServerProcessingTime(t time.Duration) {
	c.serverProcessingTime = t
}

//GetTransactionTimeOut returns the total time to wait for a transaction
//(server response) to time out, given the expected length of RTU packets.
//This function is also used internally to calculate timeout.
func (c *FailoverRTUClient) GetTransactionTimeOut(reqLen, ansLen int) time.Duration {
	l := reqLen + ansLen
	return c.com.BytesDelay(l) + c.serverProcessingTime
}

//Serve serves FailoverRTUClient side handlers, must close SerialContext after error is
//returned, to clean up.
//
//A FailoverRTUClient expects a lot of "unexpected" read packets and "lost" writes so it
//is does not do the error checking that a normal client does, but insdead try to guess the best
//interpretation.
func (c *FailoverRTUClient) Serve(handler ProtocolHandler) error {
	debugf("serve routine for %v", c.com.describe())
	go func() {
		debugf("reader routine for %v", c.com.describe())
		//Reader loop that always ready to received data. This make sure that read
		//data is always new(ish), to dump data out that is received during an
		//unexpected time.
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

	var last bytes.Buffer
	readUnexpected := func(act rtuAction, otherwise func()) {
		if act.err != nil || act.t != clientRead || len(act.data) == 0 {
			debugf("do not hand unexpected: %v", act)
			otherwise()
			return
		}
		debugf("handling unexpected: %v", act)
		pdu, err := act.data.GetPDU()
		if err != nil {
			debugf("readUnexpected GetPDU error: %v", err)
			otherwise()
			return
		}
		if !IsRequestReply(last.Bytes(), pdu) {
			if last.Len() != 0 {
				atomic.AddInt64(&c.com.Stats().OtherDrops, 1)
			}
			last.Reset()
			last.Write(pdu)
			return
		}
		defer last.Reset()

		if pdu.GetFunctionCode().IsWriteToServer() {
			//no-op for us
			return
		}

		bs, err := pdu.GetReplyValues()
		if err != nil {
			debugf("readUnexpected GetReplyValues error: %v", err)
			otherwise()
			return
		}
		err = handler.OnWrite(last.Bytes(), bs)
		if err != nil {
			debugf("readUnexpected OnWrite error: %v", err)
			otherwise()
			return
		}
	}

	for {
		act := <-c.actions
		switch act.t {
		default:
			readUnexpected(act, func() {
				atomic.AddInt64(&c.com.Stats().OtherDrops, 1)
				debugf("RTUClient drop unexpected: %v", act)
			})
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
		c.com.lock.Lock()
		active := c.com.isActive
		c.com.lock.Unlock()
		if act.data[0] == 0 || !active {
			time.Sleep(c.com.BytesDelay(len(act.data)))
			act.errChan <- nil //always success
			continue           // do not wait for read on multicast or when not active
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
					//test for read error
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
					if err == ErrorCrc {
						atomic.AddInt64(&c.com.Stats().CrcErrors, 1)
					} else {
						atomic.AddInt64(&c.com.Stats().OtherErrors, 1)
					}
					act.errChan <- err
					break READ_LOOP
				}
				hasErr, fc := rp.GetFunctionCode().SeparateError()
				if hasErr && fc == afc {
					atomic.AddInt64(&c.com.Stats().RemoteErrors, 1)
					handler.OnError(ap, rp)
					act.errChan <- fmt.Errorf("server reply with exception:%v", hex.EncodeToString(rp))
					break READ_LOOP
				}
				if !IsRequestReply(act.data.fastGetPDU(), rp) {
					readUnexpected(act, func() {
						atomic.AddInt64(&c.com.Stats().OtherErrors, 1)
					})
					act.errChan <- fmt.Errorf("unexpected reply:%v", hex.EncodeToString(rp))
					break READ_LOOP
				}
				if afc.IsReadToServer() {
					//read from server, write here
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
					act.errChan <- err //success if nil
					break READ_LOOP
				}
				act.errChan <- nil //success
				break READ_LOOP
			}
		}
	}
}

//Close closes the client and closes the connect
func (c *FailoverRTUClient) Close() error {
	return c.com.Close()
}

//DoTransaction starts a transaction, and returns a channel that returns an error
//or nil, with the default slaveID.
//
//DoTransaction is blocking.
//
//For read from server, the PDU is sent as is (after been warped up in RTU)
//For write to server, the data part given will be ignored, and filled in by data from handler.
func (c *FailoverRTUClient) DoTransaction(req PDU) error {
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
func (c *FailoverRTUClient) StartTransactionToServer(slaveID byte, req PDU, errChan chan error) {
	c.actions <- rtuAction{t: clientStart, data: MakeRTU(slaveID, req), errChan: errChan}
}
