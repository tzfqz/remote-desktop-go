//go:build !windows
// +build !windows

package input

import "fmt"

// InputController 非 Windows 存根
type InputController struct{}

func NewInputController(enableKeyboard, enableMouse bool) *InputController {
	return &InputController{}
}

func (ic *InputController) MoveMouse(x, y float64) error {
	return fmt.Errorf("not implemented on this platform")
}

func (ic *InputController) MouseClick(x, y float64, button int, down bool) error {
	return fmt.Errorf("not implemented on this platform")
}

func (ic *InputController) MouseScroll(x, y float64, deltaX, deltaY int) error {
	return fmt.Errorf("not implemented on this platform")
}

func (ic *InputController) KeyDown(keyCode int, key string) error {
	return fmt.Errorf("not implemented on this platform")
}

func (ic *InputController) KeyUp(keyCode int, key string) error {
	return fmt.Errorf("not implemented on this platform")
}

func (ic *InputController) TypeText(text string) error {
	return fmt.Errorf("not implemented on this platform")
}

func (ic *InputController) TypeChar(ch rune) error {
	return fmt.Errorf("not implemented on this platform")
}
