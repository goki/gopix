// Copyright (c) 2020, The gide / GoKi Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package imgview

import (
	"github.com/goki/gi/gi"
	"github.com/goki/gopix/picinfo"
	"github.com/goki/ki/ki"
	"github.com/goki/ki/kit"
)

// ImgView shows a bitmap image with zoom control through keyboard actions
type ImgView struct {
	gi.Bitmap
	Info *picinfo.Info `desc:"info about the image that is being viewed"`
}

var KiT_ImgView = kit.Types.AddType(&ImgView{}, ImgViewProps)

// AddNewImgView adds a new ImgView to given parent node, with given name.
func AddNewImgView(parent ki.Ki, name string) *ImgView {
	return parent.AddNewChild(KiT_ImgView, name).(*ImgView)
}

// SetInfo sets the image info
func (iv *ImgView) SetInfo(info *picinfo.Info) {
	iv.Info = info
	// todo: set image
}

var ImgViewProps = ki.Props{
	"EnumType:Flag": gi.KiT_NodeFlags,
	"max-width":     -1,
	"max-height":    -1,
}
