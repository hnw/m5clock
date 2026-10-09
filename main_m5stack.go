//go:build m5stack

package main

import (
	"machine"
	"time"

	"tinygo.org/x/drivers/ili9341"
)

func main() {
	display, err := initDisplay()
	if err != nil {
		println("display initialization failed:", err.Error())
		return
	}

	location, err := parseTimeZoneSpec(timezone)
	if err != nil {
		println("invalid -X main.timezone:", err.Error(), "- falling back to UTC")
		location = time.UTC
	}

	uiInbox := make(chan UIMessage, 4)
	go runUI(display, location, uiInbox)
	go runTimeSync(uiInbox)

	for {
		time.Sleep(time.Hour)
	}
}

func initDisplay() (*ili9341.Device, error) {
	backlight := machine.LCD_BL_PIN
	backlight.Configure(machine.PinConfig{Mode: machine.PinOutput})
	backlight.High()

	spi := machine.SPI2
	if err := spi.Configure(machine.SPIConfig{
		Frequency: 40_000_000,
		SCK:       machine.LCD_SCK_PIN,
		SDO:       machine.LCD_SDO_PIN,
		SDI:       machine.LCD_SDI_PIN,
	}); err != nil {
		return nil, err
	}
	display := ili9341.NewSPI(
		spi,
		machine.LCD_DC_PIN,
		machine.LCD_SS_PIN,
		machine.LCD_RST_PIN,
	)
	display.Configure(ili9341.Config{Width: displayWidth, Height: displayHeight})
	display.SetRotation(ili9341.Rotation0Mirror)

	return display, nil
}
