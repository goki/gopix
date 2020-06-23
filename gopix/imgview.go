// Copyright (c) 2020, The gide / GoKi Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package main

import (
	"fmt"

	"github.com/goki/gi/gi"
	"github.com/goki/gi/oswin"
	"github.com/goki/gi/oswin/key"
	"github.com/goki/gopix/imgview"
	"github.com/goki/ki/ki"
	"github.com/goki/ki/kit"
)

// ImgView is gopix version of ImgView with keyboard navigation through list of images
// and delete function.
type ImgView struct {
	imgview.ImgView
	PixView *PixView `desc:"pixview for navigating files"`
}

var KiT_ImgView = kit.Types.AddType(&ImgView{}, ImgViewProps)

// AddNewImgView adds a new ImgView to given parent node, with given name.
func AddNewImgView(parent ki.Ki, name string) *ImgView {
	return parent.AddNewChild(KiT_ImgView, name).(*ImgView)
}

func (iv *ImgView) KeyInput(kt *key.ChordEvent) {
	if gi.KeyEventTrace {
		fmt.Printf("ImgView KeyInput: %v\n", iv.PathUnique())
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
	kf := gi.KeyFun(kt.Chord())
	switch kf {
	case gi.KeyFunZoomIn:
		kt.SetProcessed()
		iv.ZoomIn()
	case gi.KeyFunZoomOut:
		kt.SetProcessed()
		iv.ZoomOut()
	case gi.KeyFunDelete, gi.KeyFunBackspace:
		kt.SetProcessed()
		iv.PixView.DeleteCurPic()
		iv.PixView.ViewRefresh() // auto next
	case gi.KeyFunMoveRight, gi.KeyFunMoveDown:
		kt.SetProcessed()
		iv.PixView.ViewNext()
	case gi.KeyFunMoveLeft, gi.KeyFunMoveUp:
		kt.SetProcessed()
		iv.PixView.ViewPrev()
	}
}

func (iv *ImgView) ConnectEvents2D() {
	iv.ImgViewEvents()
}

func (iv *ImgView) ImgViewEvents() {
	iv.ImgViewMouseEvents()
	iv.ConnectEvent(oswin.KeyChordEvent, gi.HiPri, func(recv, send ki.Ki, sig int64, d interface{}) {
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
