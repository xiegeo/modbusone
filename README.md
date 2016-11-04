# ModbusOne [![GoDoc](https://godoc.org/github.com/xiegeo/modbusone?status.svg)](https://godoc.org/github.com/xiegeo/modbusone)
A modbus library for Go, with unified client and server APIs.
One implementation to rule them all.

For usage examples, see examples/memory and handler2serial_test.go

## Why

There exists modbus libraries for Go, such as goburrow/modbus and flosse/go-modbus.
But they do not include any server APIs. Even if server function is implemented, user code will have to be written separately to support running both as client and server.

In my use case, client/server should be interchangeable. User code should worry about how to handle the translation of MODBUS data model to application logic. The only difference been the client also initiate requests.

This means that a remote function call like API, which is effective as a client API, is insufficient.

Instead, a callback based API (think http server handler) is used for both server and client.

## Development

There is no API stability, this project is in alpha.

My primary usage is RTU (over RS-485). Others may or may not be implemented in the future.

## Challenges

<strike>Working on physical layer protocol in a cross platform application layer means delay as a terminator is harder to work with.</strike> 
<strike>The driver is doing this for me.</strike>
Unless the data is bigger than the build in buffer size. Right now a hybrid approch
is used: if read is not full buffer size, then end packet; if full buffer size,
then read data for the expected length by function code and encoded length.

Compatibility with different existing Modbus implementations. (needs more testing)

Recover from transmission errors and timeouts, to work continuously unattended. (needs more testing)

Fuzze testing against crashes.

## Definitions

<dl>
<dt>Client/Server
  <dd>Also called Master/Slave in the context of serial communication.
<dt>PDU
  <dd>Protocol data unit, MODBUS application protocol, include function code and data. The same format no matter what the lower level protocol is.
<dt>ADU
  <dd>Application data unit, PDU prepended with Server addresses and postpended with error check, as needed.
<dt>RTU
  <dd>Remote terminal unit, in context of Modbus, it is a raw wire protocol delimited by delay. RTU packets are ADUs
</dl>

## License

This library is distributed under the BSD-style license found in the LICENSE file.
See also licenses folder for origins of large blocks of source code.
