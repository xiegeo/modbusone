package main

import (
	"flag"
	"fmt"
	"os"

	"github.com/goburrow/serial"
	"github.com/xiegeo/modbusone"
)

var address = flag.String("l", "", "required device location, such as: /dev/ttyS0 in linux or com1 in windows")
var baudRate = flag.Int("r", 19200, "baud rate")

var isClient = flag.Bool("c", false, "true for client, false (default) for server")
var slaveID = flag.Uint64("id", 1, "the slaveId of the server for serial communication, 0 for multicast only")
var fillData = flag.String("d", "am3", "data to start with, am3 starts memory "+
	"with bools as address (mod 3) == 0, and registers as address * 3 (mod uint16)")

var verbose = flag.Bool("v", false, "prints debugging information")

func main() {
	flag.Parse()
	if *verbose {
		modbusone.DebugOut = os.Stdout
	}
	var com = &modbusone.SerialPort{}
	err := com.Open(serial.Config{
		Address:  *address,
		BaudRate: *baudRate,
	})
	if err != nil {
		fmt.Fprintf(os.Stderr, "open serial error: %v\n", err)
		os.Exit(1)
	}
	defer com.Close()

	id, err := modbusone.Uint64ToSlaveID(*slaveID)
	if err != nil {
		fmt.Fprintf(os.Stderr, "set slaveID error: %v\n", err)
		os.Exit(1)
	}
	if *fillData == "am3" {
		fillAm3()
	}
	var device modbusone.Server
	if *isClient {
		client := modbusone.NewRTUCLient(com, id)
		//do client.StartTransaction(req PDU) in a loop in a go routine here
		device = client
	} else {
		device = modbusone.NewRTUServer(com, id)
	}
	h := modbusone.SimpleHandler{
		ReadDiscreteInputs: func(address, quantity uint16) ([]bool, error) {
			fmt.Printf("ReadDiscreteInputs from %v, quantity %v\n", address, quantity)
			return discretes[address : address+quantity], nil
		},
		WriteDiscreteInputs: func(address uint16, values []bool) error {
			fmt.Printf("WriteDiscreteInputs from %v, quantity %v\n", address, len(values))
			for i, v := range values {
				discretes[address+uint16(i)] = v
			}
			return nil
		},

		ReadCoils: func(address, quantity uint16) ([]bool, error) {
			fmt.Printf("ReadCoils from %v, quantity %v\n", address, quantity)
			return coils[address : address+quantity], nil
		},
		WriteCoils: func(address uint16, values []bool) error {
			fmt.Printf("WriteCoils from %v, quantity %v\n", address, len(values))
			for i, v := range values {
				coils[address+uint16(i)] = v
			}
			return nil
		},

		ReadInputRegisters: func(address, quantity uint16) ([]uint16, error) {
			fmt.Printf("ReadInputRegisters from %v, quantity %v\n", address, quantity)
			return inputRegisters[address : address+quantity], nil
		},
		WriteInputRegisters: func(address uint16, values []uint16) error {
			fmt.Printf("WriteInputRegisters from %v, quantity %v\n", address, len(values))
			for i, v := range values {
				inputRegisters[address+uint16(i)] = v
			}
			return nil
		},

		ReadHoldingRegisters: func(address, quantity uint16) ([]uint16, error) {
			fmt.Printf("ReadHoldingRegisters from %v, quantity %v\n", address, quantity)
			return holdingRegisters[address : address+quantity], nil
		},
		WriteHoldingRegisters: func(address uint16, values []uint16) error {
			fmt.Printf("WriteHoldingRegisters from %v, quantity %v\n", address, len(values))
			for i, v := range values {
				holdingRegisters[address+uint16(i)] = v
			}
			return nil
		},

		OnErrorImp: func(req modbusone.PDU, errRep modbusone.PDU) {
			fmt.Printf("error recived: %v from req:\n", errRep, req)
		},
	}
	err = device.Serve(&h)
	if err != nil {
		fmt.Fprintf(os.Stderr, "serve error: %v\n", err)
		os.Exit(1)
	}
}

const size = 0x10000

var discretes [size]bool
var coils [size]bool
var inputRegisters [size]uint16
var holdingRegisters [size]uint16

func fillAm3() {
	for i := range discretes {
		discretes[i] = i%3 == 0
	}
	for i := range coils {
		coils[i] = i%3 != 0
	}
	for i := range inputRegisters {
		inputRegisters[i] = uint16(i * 3)
	}
	for i := range holdingRegisters {
		holdingRegisters[i] = uint16(0xFFFF - i)
	}
}
