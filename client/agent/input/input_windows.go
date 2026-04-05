//go:build windows
// +build windows

package input

import (
	"fmt"
	"unsafe"

	"golang.org/x/sys/windows"
)

// vkCodeMap maps key names to Windows virtual key codes.
var vkCodeMap = map[string]uintptr{
	"a": 0x41, "b": 0x42, "c": 0x43, "d": 0x44, "e": 0x45,
	"f": 0x46, "g": 0x47, "h": 0x48, "i": 0x49, "j": 0x4A,
	"k": 0x4B, "l": 0x4C, "m": 0x4D, "n": 0x4E, "o": 0x4F,
	"p": 0x50, "q": 0x51, "r": 0x52, "s": 0x53, "t": 0x54,
	"u": 0x55, "v": 0x56, "w": 0x57, "x": 0x58, "y": 0x59, "z": 0x5A,
	"0": 0x30, "1": 0x31, "2": 0x32, "3": 0x33, "4": 0x34,
	"5": 0x35, "6": 0x36, "7": 0x37, "8": 0x38, "9": 0x39,
	"enter": 0x0D, "tab": 0x09, "esc": 0x1B, "space": 0x20,
	"backspace": 0x08, "delete": 0x2E,
	"left": 0x25, "up": 0x26, "right": 0x27, "down": 0x28,
	"home": 0x24, "end": 0x23, "pageup": 0x21, "pagedown": 0x22,
	"ctrl": 0x11, "shift": 0x10, "alt": 0x12, "cmd": 0x5B, "win": 0x5B,
	"f1": 0x70, "f2": 0x71, "f3": 0x72, "f4": 0x73,
	"f5": 0x74, "f6": 0x75, "f7": 0x76, "f8": 0x77,
	"f9": 0x78, "f10": 0x79, "f11": 0x7A, "f12": 0x7B,
	"[": 0xDB, "]": 0xDD, ";": 0xBA, "'": 0xDE,
	",": 0xBC, ".": 0xBE, "/": 0xBF, "\\": 0xDC,
	"-": 0xBD, "=": 0xBB,
}

const (
	INPUT_KEYBOARD         = 1
	KEYEVENTF_KEYUP        = 0x0002
	MOUSEEVENTF_LEFTDOWN   = 0x0002
	MOUSEEVENTF_LEFTUP     = 0x0004
	MOUSEEVENTF_RIGHTDOWN  = 0x0008
	MOUSEEVENTF_RIGHTUP    = 0x0010
	MOUSEEVENTF_MIDDLEDOWN = 0x0020
	MOUSEEVENTF_MIDDLEUP   = 0x0040
	MOUSEEVENTF_WHEEL      = 0x0800
	MOUSEEVENTF_HWHEEL     = 0x1000
)

// KEYBDINPUT mirrors the Windows KEYBDINPUT struct.
// Layout must match the Windows ABI (8 bytes on 32-bit, 16 on 64-bit).
type keybdInput struct {
	WVk       uint16
	ScanCode  uint16
	Flags     uint32
	Time      uint32
	ExtraInfo uintptr
}

// INPUT mirrors the Windows INPUT struct.
type input struct {
	Type uint32
	_    [4]byte // padding to align Ki
	Ki   keybdInput
}

// InputController sends mouse and keyboard events on Windows.
type InputController struct {
	enableKeyboard bool
	enableMouse   bool
}

func NewInputController(enableKeyboard, enableMouse bool) *InputController {
	return &InputController{enableKeyboard: enableKeyboard, enableMouse: enableMouse}
}

var user32 = windows.NewLazyDLL("user32.dll")

func p(name string) *windows.LazyProc {
	return user32.NewProc(name)
}

// MoveMouse moves cursor to absolute screen coordinates.
func (ic *InputController) MoveMouse(x, y float64) error {
	if !ic.enableMouse {
		return nil
	}
	r, _, _ := p("SetCursorPos").Call(uintptr(int64(x)), uintptr(int64(y)))
	if r == 0 {
		return fmt.Errorf("SetCursorPos failed")
	}
	return nil
}

// MouseClick moves to (x,y) then sends button event.
func (ic *InputController) MouseClick(x, y float64, button int, down bool) error {
	if !ic.enableMouse {
		return nil
	}
	ic.MoveMouse(x, y)
	return ic.sendMouseButton(button, down)
}

func (ic *InputController) sendMouseButton(button int, down bool) error {
	var flag uintptr
	if down {
		switch button {
		case 0: flag = MOUSEEVENTF_LEFTDOWN
		case 1: flag = MOUSEEVENTF_RIGHTDOWN
		case 2: flag = MOUSEEVENTF_MIDDLEDOWN
		}
	} else {
		switch button {
		case 0: flag = MOUSEEVENTF_LEFTUP
		case 1: flag = MOUSEEVENTF_RIGHTUP
		case 2: flag = MOUSEEVENTF_MIDDLEUP
		}
	}
	p("mouse_event").Call(flag, 0, 0, 0)
	return nil
}

// MouseScroll sends wheel events.
func (ic *InputController) MouseScroll(x, y float64, deltaX, deltaY int) error {
	if !ic.enableMouse {
		return nil
	}
	if deltaY != 0 {
		p("mouse_event").Call(MOUSEEVENTF_WHEEL, 0, 0, uintptr(deltaY*120/10))
	}
	if deltaX != 0 {
		p("mouse_event").Call(MOUSEEVENTF_HWHEEL, 0, 0, uintptr(deltaX*120/10))
	}
	return nil
}

// KeyDown presses a key.
func (ic *InputController) KeyDown(keyCode int, key string) error {
	if !ic.enableKeyboard {
		return nil
	}
	vk := uintptr(keyCode)
	if vk == 0 {
		m, ok := vkCodeMap[key]
		if !ok {
			return fmt.Errorf("unknown key: %q", key)
		}
		vk = m
	}
	p("keybd_event").Call(vk, 0, 0, 0)
	return nil
}

// KeyUp releases a key.
func (ic *InputController) KeyUp(keyCode int, key string) error {
	if !ic.enableKeyboard {
		return nil
	}
	vk := uintptr(keyCode)
	if vk == 0 {
		m, ok := vkCodeMap[key]
		if !ok {
			return fmt.Errorf("unknown key: %q", key)
		}
		vk = m
	}
	p("keybd_event").Call(vk, 0, KEYEVENTF_KEYUP, 0)
	return nil
}

// TypeText types a string.
func (ic *InputController) TypeText(text string) error {
	if !ic.enableKeyboard {
		return nil
	}
	for _, ch := range text {
		ic.TypeChar(ch)
	}
	return nil
}

// TypeChar types a single Unicode character via VkKeyScan + SendInput.
func (ic *InputController) TypeChar(ch rune) error {
	if !ic.enableKeyboard {
		return nil
	}
	ret, _, _ := p("VkKeyScanW").Call(uintptr(ch))
	vk := uint16(ret & 0xffff)
	shift := uint16((ret >> 8) & 0xff)
	if vk == 0xffff {
		return fmt.Errorf("VkKeyScan failed for %c", ch)
	}

	si := p("SendInput")

	var inputs [6]input
	n := 0

	if shift&1 == 1 {
		inputs[n] = input{Type: INPUT_KEYBOARD, Ki: keybdInput{WVk: 0x10}}
		n++
	}

	inputs[n] = input{Type: INPUT_KEYBOARD, Ki: keybdInput{WVk: vk}}
	n++
	inputs[n] = input{Type: INPUT_KEYBOARD, Ki: keybdInput{WVk: vk, Flags: KEYEVENTF_KEYUP}}
	n++

	if shift&1 == 1 {
		inputs[n] = input{Type: INPUT_KEYBOARD, Ki: keybdInput{WVk: 0x10, Flags: KEYEVENTF_KEYUP}}
		n++
	}

	si.Call(uintptr(n), uintptr(unsafe.Pointer(&inputs[0])), unsafe.Sizeof(input{}))
	return nil
}

func init() { fmt.Println("input: Windows SendInput driver loaded") }
