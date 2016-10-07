# ModbusOne
A modbus library for Go, with unified client and server APIs.
One implementation to rule them all.

## Why

There exists modbus libraries for Go, such as goburrow/modbus and flosse/go-modbus.
But they do not include any server APIs. Even if server function is implemented.
User code will have to be written separately to support running both as client and server.

In my use case, client/server should be interchangeable. User code should worry about how to handle the translation of MODBUS data model to application logic. The only difference been the client also initiate requests.

This means that a remote function call like API, which are effective as a client API, is insufficient.

Instead, a callback based API (think http server handler) is used for both server and client.

## Development

There is no API stability, this project is in alpha.

My primary usage is RTU (over RS-485).

## Challenges

Working on physical layer protocol in a cross platform application layer means delay as a terminator is harder to work with.

Compatibility with different existing Modbus implementations.

Recover from transmission errors and timeouts, to work continuously unattended.

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
