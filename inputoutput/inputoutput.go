package inputoutput

import (
	"log"
	"net/http"
	"time"

	"github.com/brycereitano/gomeboycolor/components"
	"github.com/brycereitano/gomeboycolor/constants"
	"github.com/brycereitano/gomeboycolor/types"
	"github.com/gorilla/websocket"
)

const PREFIX string = "IO"
const ROW_1 byte = 0x10
const ROW_2 byte = 0x20
const SCREEN_WIDTH int = 160
const SCREEN_HEIGHT int = 144

var upgrader = websocket.Upgrader{} // use default options

const (
	UP = iota
	DOWN
	LEFT
	RIGHT
	A
	B
	START
	SELECT
)

var KeyToIntMap = map[string]int{
	"up":     UP,
	"down":   DOWN,
	"left":   LEFT,
	"right":  RIGHT,
	"a":      A,
	"b":      B,
	"start":  START,
	"select": SELECT,
}

var ticker = time.NewTicker(time.Second / 10)

type KeyHandler struct {
	colSelect  byte
	rows       [2]byte
	irqHandler components.IRQHandler
}

func (kh *KeyHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	c, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		log.Print("upgrade:", err)
		return
	}
	defer c.Close()
	for {
		_, message, err := c.ReadMessage()
		if err != nil {
			log.Println("read:", err)
			break
		}
		log.Printf("recv: %s", message)
		key := KeyToIntMap[string(message)]
		kh.irqHandler.RequestInterrupt(constants.JOYP_HILO_IRQ)
		switch key {
		case UP:
			kh.rows[0] &= 0xB
		case DOWN:
			kh.rows[0] &= 0x7
		case LEFT:
			kh.rows[0] &= 0xD
		case RIGHT:
			kh.rows[0] &= 0xE
		case A:
			kh.rows[1] &= 0xE
		case B:
			kh.rows[1] &= 0xD
		case START:
			kh.rows[1] &= 0x7
		case SELECT:
			kh.rows[1] &= 0xB
		}
	}
}

func (k *KeyHandler) Name() string {
	return PREFIX + "-KEYB"
}

func (k *KeyHandler) Reset() {
	k.rows[0], k.rows[1] = 0x0F, 0x0F
	k.colSelect = 0x00
}

func (k *KeyHandler) LinkIRQHandler(m components.IRQHandler) {
	k.irqHandler = m
	log.Printf("%s: Linked IRQ Handler to Keyboard Handler", k.Name())
}

func (k *KeyHandler) Read(addr types.Word) byte {
	var value byte

	switch k.colSelect {
	case ROW_1:
		value = k.rows[1]
	case ROW_2:
		value = k.rows[0]
	default:
		value = 0x00
	}

	return value
}

func (k *KeyHandler) Write(addr types.Word, value byte) {
	k.colSelect = value & 0x30
}

type IO struct {
	KeyHandler          *KeyHandler
	ScreenOutputChannel chan *types.Screen
	AudioOutputChannel  chan int
}

func NewIO() *IO {
	var i *IO = new(IO)
	i.KeyHandler = new(KeyHandler)
	i.ScreenOutputChannel = make(chan *types.Screen)
	i.AudioOutputChannel = make(chan int)
	return i
}

//This will wait for updates to the display or audio and dispatch them accordingly
func (i *IO) Run() {
	for {
		select {
		case <-ticker.C:
			i.KeyHandler.Reset()
		case data := <-i.AudioOutputChannel:
			log.Println("Writing %d to audio!", data)
		}
	}
}
