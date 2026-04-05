//go:build windows

package screen

import (
	"encoding/base64"
	"fmt"
	"image"
	"image/color"
	"image/jpeg"
	"image/png"
	"runtime"
	"unsafe"

	"golang.org/x/sys/windows"
)

type CaptureGDI struct {
	width, height int
	quality     int
	hScreenDC   windows.Handle
	hMemDC      windows.Handle
	hBitmap     windows.Handle
	hOldBitmap  windows.Handle
}

func NewCaptureGDI(width, height, quality int) (*CaptureGDI, error) {
	user32 := windows.NewLazyDLL("user32.dll")
	gdi32 := windows.NewLazyDLL("gdi32.dll")

	getDC := user32.NewProc("GetDC")
	createCompatibleDC := gdi32.NewProc("CreateCompatibleDC")
	createCompatibleBitmap := gdi32.NewProc("CreateCompatibleBitmap")
	selectObject := gdi32.NewProc("SelectObject")
	getDeviceCaps := gdi32.NewProc("GetDeviceCaps")

	hScreenDC, _, _ := getDC.Call(0)
	if hScreenDC == 0 {
		return nil, fmt.Errorf("GetDC returned 0")
	}

	screenW, _, _ := getDeviceCaps.Call(hScreenDC, 118)
	screenH, _, _ := getDeviceCaps.Call(hScreenDC, 117)
	w, h := int(screenW), int(screenH)

	hMemDC, _, _ := createCompatibleDC.Call(hScreenDC)
	if hMemDC == 0 {
		return nil, fmt.Errorf("CreateCompatibleDC failed")
	}

	hBmp, _, _ := createCompatibleBitmap.Call(hScreenDC, uintptr(w), uintptr(h))
	if hBmp == 0 {
		return nil, fmt.Errorf("CreateCompatibleBitmap failed")
	}

	hOld, _, _ := selectObject.Call(hMemDC, hBmp)

	return &CaptureGDI{
		width:      w,
		height:     h,
		quality:    quality,
		hScreenDC:  windows.Handle(hScreenDC),
		hMemDC:     windows.Handle(hMemDC),
		hBitmap:    windows.Handle(hBmp),
		hOldBitmap: windows.Handle(hOld),
	}, nil
}

func (c *CaptureGDI) Capture() (string, int, int, error) {
	gdi32 := windows.NewLazyDLL("gdi32.dll")
	bitBlt := gdi32.NewProc("BitBlt")
	getDIBits := gdi32.NewProc("GetDIBits")

	bitBlt.Call(
		uintptr(c.hMemDC), 0, 0,
		uintptr(c.width), uintptr(c.height),
		uintptr(c.hScreenDC), 0, 0,
		0x00CC0020)

	hdr := struct {
		Size, Width, Height         int32
		Planes, BitCount            uint16
		Compression                 uint32
		SizeImage                   uint32
		XPelsPerMeter, YPelsPerMeter int32
		ClrUsed, ClrImportant       uint32
	}{
		Size: 40, Width: int32(c.width), Height: -int32(c.height),
		Planes: 1, BitCount: 24,
	}

	buf := make([]byte, c.width*c.height*3)
	ret, _, _ := getDIBits.Call(
		uintptr(c.hMemDC), uintptr(c.hBitmap),
		0, uintptr(c.height),
		uintptr(unsafe.Pointer(&buf[0])),
		uintptr(unsafe.Pointer(&hdr)), 0)
	if ret == 0 {
		return "", 0, 0, fmt.Errorf("GetDIBits failed")
	}

	nrgba := image.NewNRGBA(image.Rect(0, 0, c.width, c.height))
	for y := 0; y < c.height; y++ {
		row := buf[y*c.width*3 : (y+1)*c.width*3]
		for x := 0; x < c.width; x++ {
			i := x * 3
			nrgba.SetNRGBA(x, y, color.NRGBA{
				R: row[i+2], G: row[i+1], B: row[i], A: 255,
			})
		}
	}

	var imgBytes []byte
	if c.quality >= 90 {
		w := &mw{}
		png.Encode(w, nrgba)
		imgBytes = w.b
	} else {
		w := &mw{}
		jpeg.Encode(w, nrgba, &jpeg.Options{Quality: c.quality})
		imgBytes = w.b
	}

	return base64.StdEncoding.EncodeToString(imgBytes), c.width, c.height, nil
}

type mw struct{ b []byte }

func (w *mw) Write(p []byte) (int, error) { w.b = append(w.b, p...); return len(p), nil }

func (c *CaptureGDI) Close() error {
	gdi32 := windows.NewLazyDLL("gdi32.dll")
	user32 := windows.NewLazyDLL("user32.dll")
	gdi32.NewProc("DeleteObject").Call(uintptr(c.hBitmap))
	gdi32.NewProc("DeleteDC").Call(uintptr(c.hMemDC))
	user32.NewProc("ReleaseDC").Call(0, uintptr(c.hScreenDC))
	runtime.KeepAlive(c)
	return nil
}

func GetScreenSize() (int, int, error) {
	user32 := windows.NewLazyDLL("user32.dll")
	gdi32 := windows.NewLazyDLL("gdi32.dll")
	getDC := user32.NewProc("GetDC")
	getCaps := gdi32.NewProc("GetDeviceCaps")
	relDC := user32.NewProc("ReleaseDC")
	hDC, _, _ := getDC.Call(0)
	if hDC == 0 {
		return 0, 0, fmt.Errorf("GetDC failed")
	}
	defer relDC.Call(0, hDC)
	w, _, _ := getCaps.Call(hDC, 118)
	h, _, _ := getCaps.Call(hDC, 117)
	return int(w), int(h), nil
}

func init() { fmt.Println("screen: GDI BitBlt capture loaded") }
