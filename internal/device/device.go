package device

import (
	"bytes"
	"encoding/binary"
	log "github.com/Sirupsen/logrus"
	"github.com/yjiong/iotgateway/config"
	"github.com/yjiong/iotgateway/internal/common"
	"math"
	"sync"
	//	"strconv"
	//	"fmt"
	//	"strings"
)

// RegDevice ..
var RegDevice = make(Devlist)

// Commif ..
var Commif = make(map[string]string)

// Mutex ..
var Mutex = make(map[string]*sync.Mutex)

type dict map[string]interface{}

func init() {
	con, err := config.LoadConfigFile(common.CONFILEPATH)
	if err != nil {
		log.Errorf("load commif file failed : %s", err)
		return
	}
	comm, err := con.GetSection("commif")
	if err != nil {
		log.Errorf("get section commif file failed : %s", err)
		return
	}
	Commif = comm
	for ifname := range comm {
		Mutex[ifname] = new(sync.Mutex)
		//log.Info(ifname)
	}
	Mutex["usb0"] = new(sync.Mutex)
}

// Devicerwer ..
type Devicerwer interface {
	NewDev(id string, ele map[string]string) (Devicerwer, error)
	RWDevValue(rw string, m dict) (dict, error)
	CheckKey(e dict) (bool, error)
	GetElement() (dict, error)
	HelpDoc() interface{}
	//	Devid() string
}

// Device ..
type Device struct {
	devid   string
	devtype string
	commif  string
	devaddr string
	mutex   *sync.Mutex
}

// Devlist ..
type Devlist map[string]Devicerwer

// NewDevHandler ..
func NewDevHandler(devlistfile string) (map[string]Devicerwer, error) {
	con, err := config.LoadConfigFile(devlistfile)
	if err != nil {
		log.Errorf("load config file failed: %s", err)
		return nil, err
	}
	devlist := map[string]Devicerwer{}
	seclist := con.GetSectionList()
	for _, devid := range seclist {
		ele, err := con.GetSection(devid)
		if err != nil {
			log.Errorf("get %s element error : %s", devid, err)
			continue
		}
		dtype, okType := ele["_type"]
		if !okType {
			log.Errorf("get %s element type error : %s", devid, err)
			continue
		}
		if _, ok := ele["devaddr"]; !ok {
			log.Errorf("get %s element devaddr error : %s", devid, err)
			continue
		}
		if _, ok := ele["commif"]; !ok {
			log.Errorf("get %s element commif error : %s", devid, err)
			continue
		}
		if _, ok := RegDevice[dtype]; ok {
			devlist[devid], _ = RegDevice[dtype].NewDev(devid, ele)
		}
	}
	return devlist, nil
}

// NewDev ..
func (d *Device) NewDev(id string, ele map[string]string) Device {
	dmutex := new(sync.Mutex)
	return Device{
		devid:   id,
		devtype: ele["_type"],
		commif:  ele["commif"],
		devaddr: ele["devaddr"],
		mutex:   dmutex,
	}
}

// IntToBytes ..
func IntToBytes(n int) []byte {
	tmp := int32(n)
	bytesBuffer := bytes.NewBuffer([]byte{})
	binary.Write(bytesBuffer, binary.BigEndian, tmp)
	return bytesBuffer.Bytes()
}

// BytesToInt ..
func BytesToInt(b []byte) int {
	bytesBuffer := bytes.NewBuffer(b)
	var tmp int32
	binary.Read(bytesBuffer, binary.BigEndian, &tmp)
	return int(tmp)
}

// Hex2Bcd ..
func Hex2Bcd(n byte) byte {
	return IntToBytes(int(n)>>4*10 + int(n)&0x0f)[3]
}

// Bcd2Hex ..
func Bcd2Hex(n byte) byte {
	return IntToBytes((int(n)/10)<<4 + int(n)%10)[3]
}

// Bcd2_2f ..
func Bcd2_2f(a, b int) float64 {
	return float64((a>>4*10+a&0x0f)*100 + (b>>4*10 + b&0x0f))
}

// Float32ToByte ..
func Float32ToByte(float float32) []byte {
	bits := math.Float32bits(float)
	bytes := make([]byte, 4)
	binary.LittleEndian.PutUint32(bytes, bits)

	return bytes
}

//ByteToFloat32 ..
func ByteToFloat32(bytes []byte) float32 {
	bits := binary.LittleEndian.Uint32(bytes)

	return math.Float32frombits(bits)
}

// Float64ToByte ..
func Float64ToByte(float float64) []byte {
	bits := math.Float64bits(float)
	bytes := make([]byte, 8)
	binary.LittleEndian.PutUint64(bytes, bits)

	return bytes
}

// ByteToFloat64 ..
func ByteToFloat64(bytes []byte) float64 {
	bits := binary.LittleEndian.Uint64(bytes)

	return math.Float64frombits(bits)
}
