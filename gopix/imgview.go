// Copyright (c) 2020, The gide / Goki Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package main

import (
	"fmt"

	"github.com/goki/gi/gi"
	"github.com/goki/gi/oswin"
	"github.com/goki/gi/oswin/key"
	"github.com/goki/ki/ki"
	"github.com/goki/ki/kit"
	"goki.dev/gopix/imgview"
)

// ImgView is gopix version of ImgView with keyboard navigation through list of images
// and delete function.
type ImgView struct {
	imgview.ImgView

	// pixview for navigating files
	PixView *PixView
}

var KiT_ImgView = kit.Types.AddType(&ImgView{}, ImgViewProps)

// AddNewImgView adds a new ImgView to given parent node, with given name.
func AddNewImgView(parent ki.Ki, name string) *ImgView {
	return parent.AddNewChild(KiT_ImgView, name).(*ImgView)
}

func (iv *ImgView) KeyInput(kt *key.ChordEvent) {
	if gi.DebugSettings.KeyEventTrace {
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
	case "Control+R", "Meta+R":
		kt.SetProcessed()
		iv.PixView.RotateRightSel()
		iv.PixView.ViewRefresh()
	case "Control+L", "Meta+L":
		kt.SetProcessed()
		iv.PixView.RotateLeftSel()
		iv.PixView.ViewRefresh()
	}
	if kt.IsProcessed() {
		return
	}
	kf := keyfun.(kt.Chord())
	switch kf {
	case keyfun.ZoomIn:
		kt.SetProcessed()
		iv.ZoomIn()
	case keyfun.ZoomOut:
		kt.SetProcessed()
		iv.ZoomOut()
	case keyfun.Delete, keyfun.Backspace:
		kt.SetProcessed()
		iv.PixView.DeleteCurPic()
		iv.PixView.ViewRefresh() // auto next
	case keyfun.MoveRight, keyfun.MoveDown:
		kt.SetProcessed()
		iv.PixView.ViewNext()
	case keyfun.MoveLeft, keyfun.MoveUp:
		kt.SetProcessed()
		iv.PixView.ViewPrev()
	}
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

var ImgViewProps = ki.Props{
	"EnumType:Flag": gi.KiT_NodeFlags,
	"max-width":     -1,
	"max-height":    -1,
}
