# ModbusOne [![GoDoc](https://godoc.org/github.com/xiegeo/modbusone?status.svg)](https://godoc.org/github.com/xiegeo/modbusone)
A modbus library for Go, with unified client and server APIs.
One implementation to rule them all.

For usage examples, see examples/memory and handler2serial_test.go

## Why

There exists modbus libraries for Go, such as goburrow/modbus and flosse/go-modbus.
But they do not include any server APIs. Even if server function is implemented, user code will have to be written separately to support running both as client and server.

In my use case, client/server should be interchangeable. User code should worry about how to handle the translation of MODBUS data model to application logic. The only difference is the client also initiate requests.

This means that a remote function call like API, which is effective as a client side API, is insufficient.

Instead, a callback based API (like http server handler) is used for both server and client.

## Implemented
- Serial RTU
- Function Codes 1-6,15,16
- Server and Client API
- Server and Client Tester (examples/memory)

## Future
- Floating point support
- TCP (maybe)

## Development

This project is mostly stable, and I am using it in production.

API stability is best effort. This means: 

* Changles should not break users code, unless there is a compelling reason.

* Code broken by API Changles should not compile, new errors to user code should not be introduced silently. 

* API Changes will be documented to help someone losing their mind when working code stopped compiling.

My primary usage is RTU (over RS-485). Others may or may not be implemented in the future.

Contribution to new or existing functionally, or just changing a private identifier public are welcome, as well as documentation, test, example code or any other improvements. 

## Breaking Changes
2017-06-13
    Removed dependency on goburrow/serial. All serial connections should be created with NewSerialContext, which can accept any ReadWriteCloser

## Challenges

Packet separation uses a mix of length and timing indications. Length is used
if CRC is valide, otherwise timing indications is used to find where the next 
packet starts.

Compatibility with wide range of serial hardware/drivers.

Compatibility with different existing Modbus environments. (needs more testing)

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
  <dd>Remote terminal unit, in context of Modbus, it is a raw wire protocol delimited by delay. RTU is an example of ADU.
</dl>

## License

This library is distributed under the BSD-style license found in the LICENSE file.

See also licenses folder for origins of large blocks of source code.
