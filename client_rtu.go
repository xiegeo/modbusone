package modbusone

import (
	"encoding/hex"
	"errors"
	"fmt"
	"time"
)

type RTUClient struct {
	com                  SerialPort
	SlaveId              byte
	serverProcessingTime time.Duration
	actions              chan rtuAction
}

func NewRTUCLient(com SerialPort, slaveId byte) *RTUClient {
	r := RTUClient{
		com:                  com,
		SlaveId:              slaveId,
		serverProcessingTime: time.Second,
		actions:              make(chan rtuAction),
	}
	return &r
}

func (c *RTUClient) SetServerProcessingTime(t time.Duration) {
	c.serverProcessingTime = t
}

func (c *RTUClient) GetTransactionTimeOut(reqLen, ansLen int) time.Duration {
	l := reqLen + ansLen
	return RTUBytesDelay(l, c.com.BaudRate) + c.serverProcessingTime
}

type rtuAction struct {
	t       actionType
	data    RTU
	errChan chan<- error
}

var ServerTimeOutError = errors.New("server timed out")

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
	return fmt.Sprintf("%v", a)
}

//Serves RTUClient side handlers, must close transport after error is returned
//to clean up.
func (c *RTUClient) Serve(handler ProtocalHandler) error {
	delay := RTUMinDelay(c.com.BaudRate)

	var ioerr error //irrecoverable io errors
	var readerr error
	go func() {
		//Reader loop that always ready to received data. This make sure that read
		//data is always new(ish), to dump data out that is received during an
		//unexpected time.
		rb := make([]byte, MaxRTUSize)
		for {
			n, err := c.com.Read(rb)
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
		if afc.WriteToServer() {
			out, err := handler.OnRead(ap)
			if err != nil {
				sendError(act.errChan, err)
				continue
			}
			act.data = MakeRTU(act.data[0], out)
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
				sendError(act.errChan, ServerTimeOutError)
				break READ_LOOP
			case react, ok := <-c.actions:
				if !ok {
					debugf("RTUClient actions closed\n")
					return sendGetError(act.errChan)
				}
				if act.t != read {
					ioerr = fmt.Errorf("unexpected action:%s", act.t)
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
				hasErr, fc := rp.GetFunctionCode().WithoutError()
				if hasErr && fc == afc {
					handler.OnError(ap, rp)
					sendError(act.errChan, fmt.Errorf("server reply with exception:%v", hex.EncodeToString(rp)))
					break READ_LOOP
				}
				if !MatchPDU(act.data.fastGetPDU(), rp) {
					sendError(act.errChan, fmt.Errorf("unexpected reply:%v", hex.EncodeToString(rp)))
					break READ_LOOP
				}
				ec := rp.GetErrorCode()
				if ec != EcOK {
					sendError(act.errChan, ec)
					break READ_LOOP
				}
				if !afc.WriteToServer() {
					//read from server, write here
					bs, err := rp.GetReplyValues()
					if err != nil {
						sendError(act.errChan, err)
						break READ_LOOP
					}
					err = handler.OnWrite(rp, bs)
					sendError(act.errChan, err)
					break READ_LOOP
				}
				sendError(act.errChan, nil)
				break READ_LOOP
			}
		}
	}
}

func (c *RTUClient) IsClient() bool {
	return true
}

//StartTransaction starts a transaction, and returns a channel that returns an error
//or nil, with the default slaveId.
func (c *RTUClient) StartTransaction(req PDU) <-chan error {
	return c.StartTransactionToServer(c.SlaveId, req)
}

//StartTransactionToServer starts a transaction, and returns a channel that returns an error
//or nil, with a custom slaveId.
func (c *RTUClient) StartTransactionToServer(slaveId byte, req PDU) <-chan error {
	errChan := make(chan error)
	c.actions <- rtuAction{start, MakeRTU(slaveId, req), errChan}
	return errChan
}