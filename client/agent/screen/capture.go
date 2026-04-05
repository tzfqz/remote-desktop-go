//go:build !windows
// +build !windows

package screen

import (
	"fmt"
)

// CaptureGDI 非 Windows 平台的存根
type CaptureGDI struct{}

func NewCaptureGDI(width, height, quality int) (*CaptureGDI, error) {
	return nil, fmt.Errorf("GDI capture not supported on this platform")
}

func (c *CaptureGDI) Capture() (string, int, int, error) {
	return "", 0, 0, fmt.Errorf("not implemented")
}

func (c *CaptureGDI) Close() error {
	return nil
}

func GetScreenSize() (int, int, error) {
	return 1920, 1080, nil
}
