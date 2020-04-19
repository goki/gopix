// Copyright (c) 2020, The gide / GoKi Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package main

import (
	"bufio"
	"encoding/json"
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

// InfoClean cleans the info list of any blank files
func (pv *PixView) InfoClean() {
	nf := len(pv.Info)
	for i := nf - 1; i >= 0; i-- {
		tf := pv.Info[i]
		if tf == nil || tf.Thumb == "" {
			pv.Info = append(pv.Info[:i], pv.Info[i+1:]...)
		}
	}
}

// DirInfo updates Info and thumbnails based on current folder
func (pv *PixView) DirInfo() {
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
	pv.Info = make(Pics, nfl)

	ncp := runtime.NumCPU()
	nper := nfl / ncp
	st := 0
	for i := 0; i < ncp; i++ {
		ed := st + nper
		if i == ncp-1 {
			ed = nfl
		}
		go pv.InfoUpdtThr(fdir, imgs, st, ed)
		pv.WaitGp.Add(1)
		st = ed
	}
	pv.WaitGp.Wait()
	pv.InfoClean()
	pv.Info.SortByDate(true)
	pv.Thumbs = pv.Info.Thumbs()
	go pv.SaveAllInfo()
	ig := pv.ImgGrid()
	ig.SetImages(pv.Thumbs)
}

func (pv *PixView) InfoUpdtThr(fdir string, imgs []string, st, ed int) {
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
		pv.AllMu.Lock()
		pi, has := pv.AllInfo[fn]
		if has {
			pv.Info[i] = pi
			pv.AllMu.Unlock()
			continue
		}
		pv.AllMu.Unlock()
		pi, err = ReadExif(ffn)
		if err != nil {
			continue
		}
		pv.AllMu.Lock()
		pv.AllInfo[fn] = pi
		pv.AllMu.Unlock()
		pv.Info[i] = pi
		ifn, err := os.Stat(tfn)
		if err == nil {
			pi.Thumb = tfn
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
		pi.Thumb = tfn
	}
	pv.WaitGp.Done()
}

// OpenAllInfo open cached info on all pictures
func (pv *PixView) OpenAllInfo() error {
	ifn := filepath.Join(pv.ImageDir, "info.json")

	pv.AllInfo = make(map[string]*PicInfo)

	f, err := os.Open(ifn)
	defer f.Close()
	if err != nil {
		log.Println(err)
		return err
	}

	d := json.NewDecoder(f)
	err = d.Decode(&pv.AllInfo)
	if err != nil {
		log.Println(err)
	}
	fmt.Printf("open all info: %d\n", len(pv.AllInfo))
	return err
}

// SaveAllInfo save cached info on all pictures
func (pv *PixView) SaveAllInfo() error {
	ifn := filepath.Join(pv.ImageDir, "info.json")

	f, err := os.Create(ifn)
	defer f.Close()
	if err != nil {
		log.Println(err)
		return err
	}

	fb := bufio.NewWriter(f) // this makes a HUGE difference in write performance!
	defer fb.Flush()

	pv.AllMu.Lock()
	defer pv.AllMu.Unlock()

	// fmt.Printf("save all info: %d\n", len(pv.AllInfo))
	e := json.NewEncoder(fb)
	err = e.Encode(pv.AllInfo)
	if err != nil {
		log.Println(err)
	}
	// fmt.Printf("done: save all info\n")
	return err
}
