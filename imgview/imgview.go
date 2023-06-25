// Copyright (c) 2020, The gide / GoKi Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package imgview

import (
	"fmt"
	"image"

	"github.com/anthonynsimon/bild/transform"
	"github.com/goki/gi/gi"
	"github.com/goki/gi/oswin"
	"github.com/goki/gi/oswin/key"
	"github.com/goki/gi/oswin/mouse"
	"github.com/goki/gi/units"
	"github.com/goki/gopix/picinfo"
	"github.com/goki/ki/ki"
	"github.com/goki/ki/kit"
	"github.com/goki/mat32"
)

// ImgView shows a bitmap image with zoom control through keyboard actions
type ImgView struct {
	gi.Bitmap
	Info    *picinfo.Info `desc:"info about the image that is being viewed"`
	OrigImg image.Image   `desc:"cached version of original image"`
	Scale   float32       `desc:"current scale"`
}

var KiT_ImgView = kit.Types.AddType(&ImgView{}, ImgViewProps)

// AddNewImgView adds a new ImgView to given parent node, with given name.
func AddNewImgView(parent ki.Ki, name string) *ImgView {
	return parent.AddNewChild(KiT_ImgView, name).(*ImgView)
}

// SetInfo sets the image info
func (iv *ImgView) SetInfo(pi *picinfo.Info) {
	iv.SetCanFocus()
	iv.Info = pi
	var err error
	iv.OrigImg, err = pi.ImageOriented()
	if err != nil {
		return
	}
	iv.ScaleToFit()
	iv.UpdateImage()
}

// ScaleToFit sets the scale so it fits the current image
func (iv *ImgView) ScaleToFit() {
	if iv.Info == nil || iv.OrigImg == nil {
		iv.Scale = 1
		return
	}
	alc := iv.LayState.Alloc.Size.ToPoint()
	if alc == image.ZP {
		iv.Scale = 1
	} else {
		isz := iv.OrigImg.Bounds().Size()
		sx := float32(alc.X) / float32(isz.X)
		sy := float32(alc.Y) / float32(isz.Y)
		iv.Scale = mat32.Min(sx, sy)
	}
}

// ZoomIn magnifies scale of image (makes it larger)
func (iv *ImgView) ZoomIn() {
	iv.Scale += 0.1
	iv.UpdateImage()
}

// ZoomOut reduces scale of image (makes it smaller)
func (iv *ImgView) ZoomOut() {
	iv.Scale -= 0.1
	if iv.Scale < 0.01 {
		iv.Scale = 0.01
	}
	iv.UpdateImage()
}

// UpdateImage updates the image based on current scale
func (iv *ImgView) UpdateImage() {
	if iv.Info == nil || iv.OrigImg == nil {
		return
	}
	updt := iv.UpdateStart()
	defer iv.UpdateEnd(updt)

	iv.SetFullReRender()
	isz := iv.OrigImg.Bounds().Size()
	nsz := isz
	nsz.X = int(float32(isz.X) * iv.Scale)
	nsz.Y = int(float32(isz.Y) * iv.Scale)
	img := transform.Resize(iv.OrigImg, nsz.X, nsz.Y, transform.Linear)
	iv.SetImage(img, 0, 0)
	iv.SetMinPrefWidth(units.NewDot(float32(nsz.X)))
	iv.SetMinPrefHeight(units.NewDot(float32(nsz.Y)))
}

func (iv *ImgView) ConnectEvents2D() {
	iv.ImgViewEvents()
}

func (iv *ImgView) ImgViewEvents() {
	iv.ImgViewMouseEvents()
	iv.ConnectEvent(oswin.KeyChordEvent, gi.HiPri, func(recv, send ki.Ki, sig int64, d any) {
		ivv := recv.Embed(KiT_ImgView).(*ImgView)
		kt := d.(*key.ChordEvent)
		ivv.KeyInput(kt)
	})
}

func (iv *ImgView) ImgViewMouseEvents() {
	iv.ConnectEvent(oswin.MouseEvent, gi.LowRawPri, func(recv, send ki.Ki, sig int64, d any) {
		me := d.(*mouse.Event)
		ivv := recv.Embed(KiT_ImgView).(*ImgView)
		switch {
		case me.Button == mouse.Left && me.Action == mouse.DoubleClick:
			ivv.ScaleToFit()
			ivv.UpdateImage()
			me.SetProcessed()
		case me.Button == mouse.Left && me.Action == mouse.Release:
			ivv.GrabFocus()
			me.SetProcessed()
			// case me.Button == mouse.Right && me.Action == mouse.Release: // todo
			// 	igg.ItemCtxtMenu(igg.SelectedIdx)
			// 	me.SetProcessed()
		}
	})
}

func (iv *ImgView) KeyInput(kt *key.ChordEvent) {
	if gi.KeyEventTrace {
		fmt.Printf("ImgView KeyInput: %v\n", iv.Path())
	}
	switch kt.Chord() {
	case "=":
		kt.SetProcessed()
		iv.Scale = 0.01 // need to first unzoom to get orig alloc
		iv.UpdateImage()
		iv.ScaleToFit()
		iv.UpdateImage()
	case "+", "Shift++":
		kt.SetProcessed()
		iv.ZoomIn()
	case "-", "Shift+-":
		kt.SetProcessed()
		iv.ZoomOut()
	}
	if kt.IsProcessed() {
		return
	}
	kf := gi.KeyFun(kt.Chord())
	switch kf {
	case gi.KeyFunZoomIn:
		kt.SetProcessed()
		iv.ZoomIn()
	case gi.KeyFunZoomOut:
		kt.SetProcessed()
		iv.ZoomOut()
	}
}

var ImgViewProps = ki.Props{
	"EnumType:Flag": gi.KiT_NodeFlags,
	"max-width":     -1,
	"max-height":    -1,
}
