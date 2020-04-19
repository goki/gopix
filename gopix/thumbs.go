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
	"github.com/goki/gi/gi"
	"github.com/goki/ki/dirs"
)

const ThumbMaxSize = 240

// ThumbDir returns the cache dir to use for storing thumbnails
func (pv *PixView) ThumbDir() string {
	ucdir, _ := os.UserCacheDir()
	pdir := filepath.Join(ucdir, "gopix")
	pnm := filepath.Join(pdir, "thumbs")
	return pnm
}

// ThumbClean cleans the thubmnail list of any blank files
func (pv *PixView) ThumbClean() {
	nf := len(pv.Thumbs)
	for i := nf - 1; i >= 0; i-- {
		tf := pv.Thumbs[i]
		if tf == "" {
			pv.Thumbs = append(pv.Thumbs[:i], pv.Thumbs[i+1:]...)
		}
	}
}

// ThumbUpdate updates list of thumbnails based on current folder
func (pv *PixView) ThumbUpdt() {
	fdir := filepath.Join(pv.ImageDir, pv.Folder)
	tdir := pv.ThumbDir()
	os.MkdirAll(tdir, 0775)

	imgs, err := dirs.AllFiles(fdir)
	if err != nil {
		fmt.Println(err)
		return
	}
	imgs = imgs[1:] // first one is the directory itself
	nfl := len(imgs)
	pv.Thumbs = make([]string, nfl)

	ncp := runtime.NumCPU()
	nper := nfl / ncp
	st := 0
	for i := 0; i < ncp; i++ {
		ed := st + nper
		if i == ncp-1 {
			ed = nfl
		}
		go pv.ThumbUpdtThr(fdir, imgs, st, ed)
		pv.WaitGp.Add(1)
		st = ed
	}
	pv.WaitGp.Wait()
	pv.ThumbClean()
	ig := pv.ImgGrid()
	ig.SetImages(pv.Thumbs)
}

func (pv *PixView) ThumbUpdtThr(fdir string, imgs []string, st, ed int) {
	tdir := pv.ThumbDir()
	for i := st; i < ed; i++ {
		fn := filepath.Base(imgs[i])

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
		img = gi.ImageResizeMax(img, ThumbMaxSize)
		err = gi.SaveImage(tfn, img)
		if err != nil {
			log.Println(err)
		}
		pv.Thumbs[i] = tfn
	}
	pv.WaitGp.Done()
}
