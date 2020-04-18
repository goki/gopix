// Copyright (c) 2020, The gide / GoKi Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package main

import (
	"fmt"
	"log"
	"os"
	"path/filepath"
	"runtime"
	"strings"

	"github.com/anthonynsimon/bild/imgio"
	"github.com/anthonynsimon/bild/transform"
	"github.com/goki/gi/gi"
	"github.com/goki/gi/oswin"
	"github.com/goki/ki/dirs"
)

const ThumbMaxSize = 240

func (pv *PixView) ThumbDir() string {
	pdir := oswin.TheApp.AppPrefsDir()
	pnm := filepath.Join(pdir, "thumbs")
	return pnm
}

func (pv *PixView) ThumbUpdt() {
	fdir := filepath.Join(string(pv.ImageDir), pv.Folder)

	imgs, err := dirs.AllFiles(fdir)
	if err != nil {
		fmt.Println(err)
		return
	}
	pv.Images = imgs
	nfl := len(pv.Images)
	pv.Thumbs = make([]string, nfl)

	ncp := runtime.NumCPU()
	nper := nfl / ncp
	st := 0
	for i := 0; i < ncp; i++ {
		ed := st + nper
		if i == ncp-1 {
			ed = nfl
		}
		go pv.ThumbUpdtThr(fdir, st, ed)
		pv.WaitGp.Add(1)
		st = ed
	}
	pv.WaitGp.Wait()
	ig := pv.ImgGrid()
	ig.Files = pv.Thumbs
	ig.Update()
}

func (pv *PixView) ThumbUpdtThr(fdir string, st, ed int) {
	tdir := pv.ThumbDir()
	os.MkdirAll(tdir, 0775)
	for i := st; i < ed; i++ {
		fn := filepath.Base(pv.Images[i])

		ext := strings.ToLower(filepath.Ext(fn))
		if !(ext == ".jpg" || ext == ".jpeg" || ext == ".png") {
			continue
		}
		ffn := filepath.Join(fdir, fn)
		tfn := filepath.Join(tdir, fn)
		iffn, err := os.Stat(ffn)
		if err != nil {
			continue
		}
		ifn, err := os.Stat(tfn)
		if err == nil {
			pv.Thumbs[i] = tfn
			if !ifn.ModTime().Before(iffn.ModTime()) {
				continue
			}
		}
		img, err := imgio.Open(ffn)
		if err != nil {
			continue
		}
		sz := img.Bounds().Size()
		tsz := sz
		if sz.X > sz.Y {
			if tsz.X > ThumbMaxSize {
				tsz.X = ThumbMaxSize
				tsz.Y = int(float32(sz.Y) * (float32(tsz.X) / float32(sz.X)))
			}
		} else {
			if tsz.Y > ThumbMaxSize {
				tsz.Y = ThumbMaxSize
				tsz.X = int(float32(sz.X) * (float32(tsz.Y) / float32(sz.Y)))
			}
		}
		if tsz != sz {
			img = transform.Resize(img, tsz.X, tsz.Y, transform.Linear)
		}
		err = gi.SaveImage(tfn, img)
		if err != nil {
			log.Println(err)
		}
		pv.Thumbs[i] = tfn
	}
	pv.WaitGp.Done()
}
