package components

import "github.com/brycereitano/gomeboycolor/types"

type Peripheral interface {
	Name() string
	Read(Address types.Word) byte
	Write(Address types.Word, Value byte)
	LinkIRQHandler(m IRQHandler)
	Reset()
}
