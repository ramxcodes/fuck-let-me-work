//go:build !darwin || cgo

// Package tray is the menu bar / system tray app. It runs as the user and
// only talks to the daemon over the local API.
package tray

import (
	"bytes"
	"context"
	"encoding/binary"
	"image"
	"image/color"
	"image/png"
	"log"
	"runtime"
	"time"

	"fyne.io/systray"

	"github.com/ramxcodes/fuck-let-me-work/internal/api"
	"github.com/ramxcodes/fuck-let-me-work/internal/focus"
)

// Run blocks until the user picks Quit.
func Run() { systray.Run(onReady, func() {}) }

func onReady() {
	c := api.NewClient()
	iconOn, iconOff := dot(color.RGBA{0xd9, 0x48, 0x0f, 0xff}), dot(color.RGBA{0x8a, 0x84, 0x78, 0xff})
	systray.SetIcon(iconOff)
	systray.SetTooltip("fuck let me work")

	state := systray.AddMenuItem("Connecting…", "")
	state.Disable()
	systray.AddSeparator()
	starts := []struct {
		label string
		d     time.Duration
	}{{"Focus 25m", 25 * time.Minute}, {"Focus 50m", 50 * time.Minute}, {"Focus 2h", 2 * time.Hour}, {"Focus until I stop", 0}}
	startItems := make([]*systray.MenuItem, len(starts))
	for i, s := range starts {
		startItems[i] = systray.AddMenuItem(s.label, "")
	}
	stop := systray.AddMenuItem("Stop focus", "")
	systray.AddSeparator()
	panel := systray.AddMenuItem("Open control panel", "")
	quit := systray.AddMenuItem("Quit tray", "The daemon keeps running")

	render := func(st focus.Status, err error) {
		switch {
		case err != nil:
			state.SetTitle("Daemon not running")
			systray.SetIcon(iconOff)
			systray.SetTitle("")
		case st.Active:
			label := "Focus on"
			if st.Until != nil {
				label += " until " + st.Until.Local().Format("15:04")
			}
			state.SetTitle(label)
			systray.SetIcon(iconOn)
			systray.SetTitle("focus") // macOS shows this next to the icon
		default:
			state.SetTitle("Focus off")
			systray.SetIcon(iconOff)
			systray.SetTitle("")
		}
		for _, it := range startItems {
			show(it, err == nil && !st.Active)
		}
		show(stop, err == nil && st.Active)
	}
	refresh := func() {
		ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
		defer cancel()
		render(c.Status(ctx))
	}
	for i, s := range starts {
		onClick(startItems[i], refresh, func(ctx context.Context) (focus.Status, error) { return c.Start(ctx, s.d) })
	}
	onClick(stop, refresh, c.Stop)

	go func() {
		for ; ; time.Sleep(2 * time.Second) {
			refresh()
		}
	}()
	go func() {
		for range panel.ClickedCh {
			openURL(api.DefaultURL())
		}
	}()
	go func() {
		<-quit.ClickedCh
		systray.Quit()
	}()
}

// onClick runs fn on every click of it, then redraws the menu.
func onClick(it *systray.MenuItem, refresh func(), fn func(context.Context) (focus.Status, error)) {
	go func() {
		for range it.ClickedCh {
			ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			if _, err := fn(ctx); err != nil {
				log.Print(err)
			}
			cancel()
			refresh()
		}
	}()
}

func show(it *systray.MenuItem, visible bool) {
	if visible {
		it.Show()
	} else {
		it.Hide()
	}
}

// dot draws a filled circle icon. Windows needs ICO, the others take PNG.
func dot(c color.RGBA) []byte {
	const size = 32
	img := image.NewRGBA(image.Rect(0, 0, size, size))
	for y := range size {
		for x := range size {
			dx, dy := float64(x)-15.5, float64(y)-15.5
			if dx*dx+dy*dy <= 11*11 {
				img.Set(x, y, c)
			}
		}
	}
	var p bytes.Buffer
	png.Encode(&p, img)
	if runtime.GOOS != "windows" {
		return p.Bytes()
	}
	// ICO with one PNG image inside (supported since Windows Vista).
	var ico bytes.Buffer
	binary.Write(&ico, binary.LittleEndian, []uint16{0, 1, 1})                 // reserved, type=icon, count
	ico.Write([]byte{size, size, 0, 0})                                        // width, height, colors, reserved
	binary.Write(&ico, binary.LittleEndian, []uint16{1, 32})                   // planes, bits per pixel
	binary.Write(&ico, binary.LittleEndian, []uint32{uint32(p.Len()), 6 + 16}) // data size, data offset
	ico.Write(p.Bytes())
	return ico.Bytes()
}
