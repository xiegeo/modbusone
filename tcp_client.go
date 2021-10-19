package modbusone

import (
	"context"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"sync"
)

//TCPClient implements Client/Master side logic for Modbus over a TCP connection to
//be used by a ProtocolHandler.
type TCPClient struct {
	ctx       context.Context
	cancle    context.CancelFunc
	conn      io.ReadWriteCloser
	SlaveID   byte
	handler   ProtocalHandler
	exitError error //set this before call to cancle
	locker    sync.Mutex
}

//TCPClient is also a Server
var _ Server = &TCPClient{}

//NewTCPClient create a new client communicating over a TCP connetion with the
//given slaveID as default.
func NewTCPClient(conn io.ReadWriteCloser, slaveID byte) *TCPClient {
	ctx, cancle := context.WithCancel(context.Background())
	return &TCPClient{
		ctx:     ctx,
		cancle:  cancle,
		conn:    conn,
		SlaveID: slaveID,
	}
}

//Serve serves TCPClient handlers.
func (c *TCPClient) Serve(handler ProtocolHandler) error {
	defer c.Close()
	c.locker.Lock()
	c.handler = handler
	c.locker.Unlock()
	<-c.ctx.Done()
	if c.exitError != nil {
		return c.exitError
	}
	return c.ctx.Err()
}

//Close closes the client and closes the TCP connection
func (c *TCPClient) Close() error {
	if c.exitError == nil {
		c.exitError = errors.New("closed by user action")
	}
	c.cancle()
	return c.conn.Close()
}

//DoTransaction starts a transaction, and returns a channel that returns an error
//or nil, with the default slaveID.
//
//DoTransaction is blocking.
//
//For read from server, the PDU is sent as is (after been warped up in RTU)
//For write to server, the data part given will be ignored, and filled in by data from handler.
func (c *TCPClient) DoTransaction(req PDU) error {
	return c.DoTransaction2(c.SlaveID, req)
}

//DoTransaction2 is DoTransaction with a settable slaveID.
func (c *TCPClient) DoTransaction2(slaveID byte, req PDU) error {
	c.locker.Lock() //only handle one transction at a time for now
	defer c.locker.Unlock()
	var bs []byte
	if OverSizeSupport {
		bs = make([]byte, OverSizeMaxRTU+TCPHeaderLength)
	} else {
		bs = make([]byte, MaxRTUSize+TCPHeaderLength)
	}
	if req.GetFunctionCode().IsWriteToServer() {
		data, err := c.handler.OnRead(req)
		if err != nil {
			return err
		}
		req = req.MakeWriteRequest(data)
	}
	_, err := writeTCP(c.conn, bs, req)
	if err != nil {
		c.exitError = err
		c.cancle()
		return err
	}
	n, err := readTCP(c.conn, bs)
	if err != nil {
		c.exitError = err
		c.cancle()
		return err
	}
	rp := PDU(bs[MBAPHeaderLength:n])
	hasErr, fc := rp.GetFunctionCode().SeparateError()
	if hasErr {
		c.handler.OnError(req, rp)
		return fmt.Errorf("server reply with exception:%v", hex.EncodeToString(rp))
	}
	if !IsRequestReply(req, rp) {
		err = errors.New("unexpected packet received")
		c.exitError = err
		c.cancle()
		return err
	}
	if fc.IsReadToServer() {
		//read from server, write here
		bs, err := rp.GetReplyValues()
		if err != nil {
			c.exitError = err
			c.cancle()
			return err
		}
		return c.handler.OnWrite(req, bs)
	}
	return nil
}

//StartTransactionToServer starts a transaction, with a custom slaveID.
//errChan is required, an error is set if the transaction failed, or
//nil for success.
//
//StartTransactionToServer is not blocking.
//
//For read from server, the PDU is sent as is (after been warped up in RTU).
//For write to server, the data part given will be ignored, and filled in by data from handler.
func (c *TCPClient) StartTransactionToServer(slaveID byte, req PDU, errChan chan error) {
	go func() {
		errChan <- func() (err error) {
			defer func() {
				if r := recover(); r != nil {
					err = fmt.Errorf("panic: %v", r)
				}
			}()
			return c.DoTransaction2(slaveID, req)
		}()
	}()
}
