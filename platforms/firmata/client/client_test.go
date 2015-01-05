package client

import (
	"testing"
	"time"

	"github.com/hybridgroup/gobot"
)

type readWriteCloser struct{}

var testLastWriteData = []byte{}

func (readWriteCloser) Write(p []byte) (int, error) {
	testLastWriteData = p
	return len(p), nil
}

var testReadData = []byte{}

func (readWriteCloser) Read(b []byte) (int, error) {
	size := len(b)
	if len(testReadData) < size {
		size = len(testReadData)
	}
	copy(b, []byte(testReadData)[:size])
	testReadData = testReadData[size:]

	return size, nil
}

func (readWriteCloser) Close() error {
	return nil
}

func initTestFirmata() *Client {
	b := New(readWriteCloser{})
	testProtocolResponse()
	b.process()
	testFirmwareResponse()
	b.process()
	testCapabilitiesResponse()
	b.process()
	testAnalogMappingResponse()
	b.process()
	return b
}

func testProtocolResponse() {
	// arduino uno r3 protocol response "2.3"
	testReadData = []byte{249, 2, 3}
}

func testFirmwareResponse() {
	// arduino uno r3 firmware response "StandardFirmata.ino"
	testReadData = []byte{240, 121, 2, 3, 83, 0, 116, 0, 97, 0, 110, 0, 100, 0, 97,
		0, 114, 0, 100, 0, 70, 0, 105, 0, 114, 0, 109, 0, 97, 0, 116, 0, 97, 0, 46,
		0, 105, 0, 110, 0, 111, 0, 247}
}

func testCapabilitiesResponse() {
	// arduino uno r3 capabilities response
	testReadData = []byte{240, 108, 127, 127, 0, 1, 1, 1, 4, 14, 127, 0, 1, 1, 1, 3,
		8, 4, 14, 127, 0, 1, 1, 1, 4, 14, 127, 0, 1, 1, 1, 3, 8, 4, 14, 127, 0, 1,
		1, 1, 3, 8, 4, 14, 127, 0, 1, 1, 1, 4, 14, 127, 0, 1, 1, 1, 4, 14, 127, 0,
		1, 1, 1, 3, 8, 4, 14, 127, 0, 1, 1, 1, 3, 8, 4, 14, 127, 0, 1, 1, 1, 3, 8,
		4, 14, 127, 0, 1, 1, 1, 4, 14, 127, 0, 1, 1, 1, 4, 14, 127, 0, 1, 1, 1, 2,
		10, 127, 0, 1, 1, 1, 2, 10, 127, 0, 1, 1, 1, 2, 10, 127, 0, 1, 1, 1, 2, 10,
		127, 0, 1, 1, 1, 2, 10, 6, 1, 127, 0, 1, 1, 1, 2, 10, 6, 1, 127, 247}
}

func testAnalogMappingResponse() {
	// arduino uno r3 analog mapping response
	testReadData = []byte{240, 106, 127, 127, 127, 127, 127, 127, 127, 127, 127, 127,
		127, 127, 127, 127, 0, 1, 2, 3, 4, 5, 247}
}

func TestReportVersion(t *testing.T) {
	b := initTestFirmata()
	//test if functions executes
	b.QueryProtocolVersion()
}

func TestQueryFirmware(t *testing.T) {
	b := initTestFirmata()
	//test if functions executes
	b.QueryFirmware()
}

func TestQueryPinState(t *testing.T) {
	b := initTestFirmata()
	//test if functions executes
	b.QueryPinState(1)
}

func TestProcess(t *testing.T) {
	b := initTestFirmata()

	sem := make(chan bool)
	//ProtocolVersion
	gobot.Once(b.Event("ProtocolVersion"), func(data interface{}) {
		gobot.Assert(t, data.(string), "2.3")
		sem <- true
	})

	testProtocolResponse()
	go b.process()

	select {
	case <-sem:
	case <-time.After(10 * time.Millisecond):
		t.Errorf("ProtocolVersion was not published")
	}

	//AnalogMessageRangeStart
	gobot.Once(b.Event("AnalogRead0"), func(data interface{}) {
		gobot.Assert(t, data.(int), 675)
		sem <- true
	})

	testReadData = []byte{0xE0, 0x23, 0x05}
	go b.process()

	select {
	case <-sem:
	case <-time.After(10 * time.Millisecond):
		t.Errorf("AnalogRead0 was not published")
	}

	gobot.Once(b.Event("AnalogRead1"), func(data interface{}) {
		gobot.Assert(t, data.(int), 803)
		sem <- true
	})
	testReadData = []byte{0xE1, 0x23, 0x06}

	go b.process()

	select {
	case <-sem:
	case <-time.After(10 * time.Millisecond):
		t.Errorf("AnalogRead1 was not published")
	}

	//digitalMessageRangeStart
	b.Pins[2].Mode = Input
	gobot.Once(b.Event("DigitalRead2"), func(data interface{}) {
		gobot.Assert(t, data.(int), 1)
		sem <- true
	})

	testReadData = []byte{0x90, 0x04, 0x00}
	go b.process()

	select {
	case <-sem:
	case <-time.After(10 * time.Millisecond):
		t.Errorf("DigitalRead2 was not published")
	}

	b.Pins[4].Mode = Input
	gobot.Once(b.Event("DigitalRead4"), func(data interface{}) {
		gobot.Assert(t, data.(int), 1)
		sem <- true
	})

	testReadData = []byte{0x90, 0x16, 0x00}
	go b.process()

	select {
	case <-sem:
	case <-time.After(10 * time.Millisecond):
		t.Errorf("DigitalRead4 was not published")
	}

	//pinStateResponse
	gobot.Once(b.Event("PinState13"), func(data interface{}) {
		gobot.Assert(t, data.(PinState), PinState{
			Pin:   13,
			Mode:  1,
			Value: 1,
		})
		sem <- true
	})
	testReadData = []byte{240, 110, 13, 1, 1, 247}

	go b.process()

	select {
	case <-sem:
	case <-time.After(10 * time.Millisecond):
		t.Errorf("PinState13 was not published")
	}

	//i2cReply
	gobot.Once(b.Event("I2cReply"), func(data interface{}) {
		//response := I2cResponse{
		reply := I2cReply{
			Address:  9,
			Register: 0,
			Data:     []byte{152, 1, 154},
		}
		gobot.Assert(t, data.(I2cReply), reply)
		sem <- true
	})

	testReadData = []byte{240, 119, 9, 0, 0, 0, 24, 1, 1, 0, 26, 1, 247}
	go b.process()

	select {
	case <-sem:
	case <-time.After(10 * time.Millisecond):
		t.Errorf("I2cReply was not published")
	}

	//firmwareName
	gobot.Once(b.Event("FirmwareQuery"), func(data interface{}) {
		gobot.Assert(t, data.(string), "StandardFirmata.ino")
		sem <- true
	})

	testReadData = []byte{240, 121, 2, 3, 83, 0, 116, 0, 97, 0, 110, 0, 100, 0, 97,
		0, 114, 0, 100, 0, 70, 0, 105, 0, 114, 0, 109, 0, 97, 0, 116, 0, 97, 0, 46,
		0, 105, 0, 110, 0, 111, 0, 247}
	go b.process()
	select {
	case <-sem:
	case <-time.After(10 * time.Millisecond):
		t.Errorf("FirmwareQuery was not published")
	}

	//stringData
	gobot.Once(b.Event("StringData"), func(data interface{}) {
		gobot.Assert(t, data.(string), "Hello Firmata!")
		sem <- true
	})
	testReadData = append([]byte{240, 0x71},
		append([]byte("Hello Firmata!"), 247)...)
	go b.process()

	select {
	case <-sem:
	case <-time.After(10 * time.Millisecond):
		t.Errorf("StringData was not published")
	}
}
