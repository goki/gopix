// Copyright (c) 2020, The gide / GoKi Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package imgrid

import (
	"image"

	"github.com/goki/gi/gi"
	"github.com/goki/gi/units"
	"github.com/goki/ki/ints"
	"github.com/goki/ki/ki"
	"github.com/goki/ki/kit"
	"github.com/goki/mat32"
)

// ImgGrid shows a list of images in a grid.
// The outer layout contains the inner grid and a scrollbar
type ImgGrid struct {
	gi.Frame
	Size  image.Point `desc:"number of columns and rows to display"`
	Files []string    `desc:"list of image files to display"`
}

var KiT_ImgGrid = kit.Types.AddType(&ImgGrid{}, nil)

// AddNewImgGrid adds a new imggrid to given parent node, with given name.
func AddNewImgGrid(parent ki.Ki, name string) *ImgGrid {
	return parent.AddNewChild(KiT_ImgGrid, name).(*ImgGrid)
}

// Config configures the grid
func (ig *ImgGrid) Config() {
	updt := ig.UpdateStart()
	defer ig.UpdateEnd(updt)

	if !ig.HasChildren() {
		ig.Lay = gi.LayoutHoriz
		gi.AddNewLayout(ig, "grid", gi.LayoutGrid)
		sbb := gi.AddNewScrollBar(ig, "sb")
		sbb.SliderSig.Connect(ig.This(), func(recv, send ki.Ki, sig int64, data interface{}) {
			igg := recv.(*ImgGrid)
			if sig == int64(gi.SliderValueChanged) {
				igg.Update()
			}
		})
	}
	gr := ig.Child(0).(*gi.Layout)
	sb := ig.Child(1).(*gi.ScrollBar)
	if ig.Size.X == 0 {
		ig.Size.X = 4
		ig.Size.Y = 4
	}
	gr.SetProp("columns", ig.Size.X)
	gr.Lay = gi.LayoutGrid
	gr.SetStretchMax()
	gr.SetProp("spacing", gi.StdDialogVSpaceUnits)
	ng := ig.Size.X * ig.Size.Y
	if ng != gr.NumChildren() {
		gr.SetNChildren(ng, gi.KiT_Bitmap, "b_")
	}
	nf := len(ig.Files)
	nr := nf / ig.Size.X
	nr = ints.MaxInt(nr, ig.Size.Y)
	sb.Defaults()
	sb.Min = 0
	sb.Max = float32(nr)
	sb.ThumbVal = float32(ig.Size.Y)
	sb.Tracking = true
	sb.Dim = mat32.Y
	sb.SetFixedWidth(units.NewPx(16))
	sb.SetStretchMaxHeight()
}

// Update updates the display for given scrollbar position, rendering the images
func (ig *ImgGrid) Update() {
	updt := ig.UpdateStart()
	defer ig.UpdateEnd(updt)

	gr := ig.Child(0).(*gi.Layout)
	sb := ig.Child(1).(*gi.ScrollBar)

	nf := len(ig.Files)
	nr := nf / ig.Size.X
	nr = ints.MaxInt(nr, ig.Size.Y)
	sb.Max = float32(nr)
	// fmt.Printf("nf: %v  nr: %v\n", nf, nr)

	bimg := image.NewNRGBA(image.Rect(0, 0, 50, 50))
	si := int(sb.Value)
	idx := si * ig.Size.X
	bi := 0
	for y := 0; y < ig.Size.Y; y++ {
		for x := 0; x < ig.Size.X; x++ {
			bm := gr.Child(bi).(*gi.Bitmap)
			if idx < nf {
				f := ig.Files[idx]
				if f != "" {
					bm.OpenImage(gi.FileName(f), 0, 0)
				} else {
					bm.SetImage(bimg, 0, 0)
				}
			} else {
				bm.SetImage(bimg, 0, 0)
			}
			bm.LayoutToImgSize()
			bi++
			idx++
		}
	}
}

// func (ig *ImgGrid) Render2D() {
// 	// pv.ToolBar().UpdateActions()
// 	ig.Update()
// 	ig.Layout.Render2D()
// }
