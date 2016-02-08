package bme280

import (
	//"fmt"
	"github.com/kidoman/embd"
	"sync"
)

const (
	DeviceAddr      = 0x76
	CtrlHumAddr     = 0xf2
	CtrlMeasureAddr = 0xf4
	CtrlConfAddr    = 0xf5
)

type BME280Opt struct {
	TemperatureOverSampling uint
	PressureOverSampling    uint
	HumidityOverSampling    uint
	Mode                    uint
	TStandby                uint
	Filter                  uint
	SPI3WEnable             bool
}

func NewOpt() *BME280Opt {
	return &BME280Opt{
		TemperatureOverSampling: 1, // Temperature oversampling x 1
		PressureOverSampling:    1, // Pressure oversampling x 1
		HumidityOverSampling:    1, // Humidity oversampling x 1
		Mode:                    3, // Normal mode
		TStandby:                5, // 1000 msec
		Filter:                  0, // off
		SPI3WEnable:             false,
	}
}

func (o *BME280Opt) MeasureReg() byte {
	return byte((o.TemperatureOverSampling << 5) | (o.PressureOverSampling << 2) | o.Mode)
}

func (o *BME280Opt) ConfigReg() byte {
	var spi3wEnable uint
	if o.SPI3WEnable {
		spi3wEnable = 1
	} else {
		spi3wEnable = 0
	}
	return byte((o.TStandby << 5) | (o.Filter << 2) | spi3wEnable)
}

type BME280 struct {
	Bus        embd.I2CBus
	Opt        *BME280Opt
	cmu        sync.RWMutex
	calibrated bool
	calibval   []byte
	tfine      int32
	digT1      uint16
	digT2      int16
	digT3      int16
	digP1      uint16
	digP2      int16
	digP3      int16
	digP4      int16
	digP5      int16
	digP6      int16
	digP7      int16
	digP8      int16
	digP9      int16
	digH1      uint8
	digH2      int16
	digH3      uint8
	digH4      int16
	digH5      int16
	digH6      int8
}

func New(bus embd.I2CBus, opt *BME280Opt) (*BME280, error) {
	bme280 := &BME280{Bus: bus, Opt: opt}
	if err := bme280.setup(); err != nil {
		return nil, err
	}
	if err := bme280.callibrate(); err != nil {
		return nil, err
	}
	return bme280, nil
}

func (d *BME280) setup() error {
	regs := []byte{byte(d.Opt.HumidityOverSampling), d.Opt.MeasureReg(), d.Opt.ConfigReg()}
	addrs := []byte{CtrlHumAddr, CtrlMeasureAddr, CtrlConfAddr}

	for i := 0; i <= 2; i = i + 1 {
		if err := d.Bus.WriteByteToReg(DeviceAddr, addrs[i], regs[i]); err != nil {
			return err
		}
	}
	return nil
}

func (d *BME280) calibrateTemp() error {
	d.digT1 = uint16(uint16(d.calibval[1])<<8 | uint16(d.calibval[0]))
	d.digT2 = int16(int16(d.calibval[3])<<8 | int16(d.calibval[2]))
	d.digT3 = int16(int16(d.calibval[5])<<8 | int16(d.calibval[4]))
	// fmt.Printf("T 0x%x 0x%x 0x%x 0x%x\n", calib[2], calib[3], calib[4], calib[5])
	return nil
}

func (d *BME280) calibratePres() error {
	d.digP1 = uint16(uint16(d.calibval[7])<<8 | uint16(d.calibval[6]))
	d.digP2 = int16(int16(d.calibval[9])<<8 | int16(d.calibval[8]))
	d.digP3 = int16(int16(d.calibval[11])<<8 | int16(d.calibval[10]))
	d.digP4 = int16(int16(d.calibval[13])<<8 | int16(d.calibval[12]))
	d.digP5 = int16(int16(d.calibval[15])<<8 | int16(d.calibval[14]))
	d.digP6 = int16(int16(d.calibval[17])<<8 | int16(d.calibval[16]))
	d.digP7 = int16(int16(d.calibval[19])<<8 | int16(d.calibval[18]))
	d.digP8 = int16(int16(d.calibval[21])<<8 | int16(d.calibval[20]))
	d.digP9 = int16(int16(d.calibval[23])<<8 | int16(d.calibval[22]))
	// fmt.Printf("P %#v %#v %#v %#v %#v %#v %#v %#v %#v\n", d.digP1, d.digP2, d.digP3, d.digP4, d.digP5, d.digP6, d.digP7, d.digP8, d.digP9)
	return nil
}

func (d *BME280) calibrateHum() error {
	d.digH2 = int16(d.calibval[1])<<8 | int16(d.calibval[0])
	d.digH3 = uint8(d.calibval[2])
	d.digH4 = int16(d.calibval[3])<<4 | (0x0f & int16(d.calibval[4]))
	d.digH5 = int16(d.calibval[5])<<4 | (int16(d.calibval[4]) >> 4)
	d.digH6 = int8(d.calibval[6])
	// fmt.Printf("H %#v %#v %#v %#v %#v %#v\n", d.digH1, d.digH2, d.digH3, d.digH4, d.digH5, d.digH6)
	return nil
}

func (d *BME280) callibrate() error {
	d.cmu.RLock()
	if d.calibrated {
		d.cmu.RUnlock()
		return nil
	}
	d.cmu.RUnlock()

	d.cmu.Lock()
	defer d.cmu.Unlock()

	d.calibval = make([]byte, 26)
	if err := d.Bus.ReadFromReg(DeviceAddr, byte(0x88), d.calibval); err != nil {
		return err
	}

	if err := d.calibrateTemp(); err != nil {
		return err
	}
	if err := d.calibratePres(); err != nil {
		return err
	}
	d.digH1 = uint8(d.calibval[25])
	d.calibval = make([]byte, 7)
	if err := d.Bus.ReadFromReg(DeviceAddr, byte(0xe1), d.calibval); err != nil {
		return err
	}
	if err := d.calibrateHum(); err != nil {
		return err
	}

	d.calibrated = true

	return nil
}

func (d *BME280) compensateTemp(raw int32) float64 {
	t1 := float64(d.digT1)
	t2 := float64(d.digT2)
	t3 := float64(d.digT3)
	raw64 := float64(raw)

	v1 := (raw64/16384.0 - t1/1024.0) * t2
	v2 := (raw64/131072.0 - t1/8192.0) * (raw64/131072.0 - t1/8192.0) * t3
	tfine := v1 + v2
	d.tfine = int32(tfine)

	return tfine / 5120.0
}

func (d *BME280) compensatePres(raw int32) float64 {
	p1 := float64(d.digP1)
	p2 := float64(d.digP2)
	p3 := float64(d.digP3)
	p4 := float64(d.digP4)
	p5 := float64(d.digP5)
	p6 := float64(d.digP6)
	p7 := float64(d.digP7)
	p8 := float64(d.digP8)
	p9 := float64(d.digP9)
	raw64 := float64(raw)

	pres := 1048576.0 - raw64

	v1 := float64(d.tfine)/2.0 - 64000.0
	v2 := v1 * v1 * p6 / 32768.0
	v2 = v2 + v1*p5*2.0
	v2 = (v2 / 4.0) + (p4 * 65536.0)
	v1 = (p3*v1*v1/524288.0 + p2*v1) / 524288.0
	v1 = (1.0 + v1/32768.0) * p1

	/* Avoid exception caused by division by zero */
	if v1 != 0.0 {
		pres = (pres - (v2 / 4096.0)) * 6250.0 / v1
	} else {
		return 0.0 // invalid
	}
	v1 = p9 * pres * pres / 2147483648.0
	v2 = pres * p8 / 32768.0
	pres = pres + (v1+v2+p7)/16.0

	return pres
}

func (d *BME280) compensateHum(raw int32) float64 {
	hum := float64(d.tfine) - 76800.0

	h1 := float64(d.digH1)
	h2 := float64(d.digH2)
	h3 := float64(d.digH3)
	h4 := float64(d.digH4)
	h5 := float64(d.digH5)
	h6 := float64(d.digH6)

	raw64 := float64(raw)

	if hum != 0.0 {
		hum = (raw64 - (h4*64.0 + h5/16384.0*hum)) * (h2 / 65536.0 * (1.0 + h6/67108864.0*hum*(1.0+h3/67108864.0*hum)))
	} else {
		return 0.0 // invalid
	}

	hum = hum * (1.0 - h1*hum/524288.0)
	if hum > 100.0 {
		hum = 100.0
	} else if hum < 0.0 {
		hum = 0.0
	}

	return hum
}

func (d *BME280) Read() ([]float64, error) {
	data := make([]byte, 8)
	if err := d.Bus.ReadFromReg(DeviceAddr, byte(0xf7), data); err != nil {
		return nil, err
	}
	presRaw := (int32(data[0]) << 12) | (int32(data[1]) << 4) | (int32(data[2]) >> 4)
	tempRaw := (int32(data[3]) << 12) | (int32(data[4]) << 4) | (int32(data[5]) >> 4)
	humRaw := (int32(data[6]) << 8) | int32(data[7])

	d.tfine = 0 // initialize

	temp := d.compensateTemp(tempRaw)
	pres := d.compensatePres(presRaw)
	hum := d.compensateHum(humRaw)

	return []float64{temp, pres, hum}, nil
}
