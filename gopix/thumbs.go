// Copyright (c) 2020, The gide / GoKi Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package main

import (
	"bufio"
	"encoding/json"
	"fmt"
	"image"
	"log"
	"os"
	"path/filepath"
	"runtime"
	"strings"

	"github.com/anthonynsimon/bild/imgio"
	"github.com/goki/gi/gi"
	"github.com/goki/ki/dirs"
	"github.com/goki/mat32"
	"github.com/jdeng/goheif"
)

const ThumbMaxSize = 256

// ThumbDir returns the cache dir to use for storing thumbnails
func (pv *PixView) ThumbDir() string {
	ucdir, _ := os.UserCacheDir()
	pdir := filepath.Join(ucdir, "GoPix")
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

	// fmt.Printf("starting...\n")
	imgs, err := dirs.AllFiles(fdir)
	if err != nil {
		fmt.Println(err)
		return
	}
	imgs = imgs[1:] // first one is the directory itself
	nfl := len(imgs)
	pv.Info = make(Pics, nfl)

	// fmt.Printf("N files %d\n", nfl)

	// first pass fill in from existing info -- no locking
	for i := nfl - 1; i >= 0; i-- {
		fn := filepath.Base(imgs[i])
		ext := strings.ToLower(filepath.Ext(fn))
		if !(ext == ".jpg" || ext == ".jpeg" || ext == ".png" || ext == ".gif" || ext == ".heic") {
			imgs = append(imgs[:i], imgs[i+1:]...)
			pv.Info = append(pv.Info[:i], pv.Info[i+1:]...)
			continue
		}
		pi, has := pv.AllInfo[fn]
		if has {
			pv.Info[i] = pi
		}
	}

	nfl = len(imgs)
	// fmt.Printf("First pass done, now N files %d\n", nfl)

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
	// fmt.Printf("second pass done\n")
	pv.Info.SortByDate(true)
	// fmt.Printf("sort done\n")
	pv.Thumbs = pv.Info.Thumbs()
	go pv.SaveAllInfo()
	ig := pv.ImgGrid()
	ig.SetImages(pv.Thumbs)
	// fmt.Printf("done\n")
}

func (pv *PixView) InfoUpdtThr(fdir string, imgs []string, st, ed int) {
	dreg := image.Rect(5, 5, 100, 25)
	tdir := pv.ThumbDir()
	for i := st; i < ed; i++ {
		if pv.Info[i] != nil {
			continue
		}
		fn := filepath.Base(imgs[i])
		ext := filepath.Ext(fn)
		fnext := strings.TrimSuffix(fn, ext)
		ffn := filepath.Join(fdir, fn)
		tfn := filepath.Join(tdir, fnext+".jpg")
		iffn, _ := os.Stat(ffn)
		pi, err := ReadExif(ffn)
		if pi == nil {
			fmt.Printf("failed exif: %v err: %v\n", fn, err)
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
		img, err := OpenImage(ffn)
		if err != nil {
			continue
		}
		img = gi.ImageResizeMax(img, ThumbMaxSize)
		isz := img.Bounds().Size()
		rgb := img.(*image.RGBA)
		tr := &gi.TextRender{}
		rs := &gi.RenderState{}
		rs.Init(isz.X, isz.Y, rgb)
		rs.Bounds.Max = isz
		ds := pi.DateTaken.Format("2006:01:02")
		avg := AvgImgGrey(rgb, dreg)
		// fmt.Printf("img: %v  avg: %v\n", fn, avg)
		if avg < .5 {
			pv.Sty.Font.Color.SetUInt8(0xff, 0xff, 0xff, 0xff)
		} else {
			pv.Sty.Font.Color.SetUInt8(0, 0, 0, 0xff)
		}
		tr.SetString(ds, &pv.Sty.Font, &pv.Sty.UnContext, &pv.Sty.Text, true, 0, 1)
		tr.RenderTopPos(rs, mat32.Vec2{5, 5})

		err = gi.SaveImage(tfn, rgb)
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

	fmt.Printf("Loading All photos info\n")
	d := json.NewDecoder(f)
	err = d.Decode(&pv.AllInfo)
	if err != nil {
		log.Println(err)
	}
	fmt.Printf("%d Pictures Loaded\n", len(pv.AllInfo))
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

// OpenImage handles all image formats including jpeg or HEIC
func OpenImage(fnm string) (image.Image, error) {
	ext := strings.ToLower(filepath.Ext(fnm))
	if ext == ".jpg" || ext == ".jpeg" || ext == ".png" || ext == ".gif" {
		img, err := imgio.Open(fnm)
		if err != nil {
			log.Println(err)
		}
		return img, err
	}
	if ext == ".heic" {
		img, err := OpenHEIC(fnm)
		if err != nil {
			log.Println(err)
		}
		return img, err
	}
	return nil, fmt.Errorf("unsupported image file type: %s", ext)
}

// OpenHEIC opens a HEIC formatted file
func OpenHEIC(fnm string) (image.Image, error) {
	f, err := os.Open(fnm)
	if err != nil {
		return nil, err
	}
	defer f.Close()

	img, err := goheif.Decode(f)
	if err != nil {
		return nil, err
	}

	return img, nil
}

// AvgImgGrey returns the average image intensity (greyscale value) in given region
func AvgImgGrey(img *image.RGBA, reg image.Rectangle) float32 {
	reg = reg.Intersect(img.Bounds())
	gsum := float32(0)
	cnt := 0
	for y := reg.Min.Y; y < reg.Max.Y; y++ {
		for x := reg.Min.X; x < reg.Max.X; x++ {
			pos := y*img.Stride + x*4
			gsum += float32(img.Pix[pos+0]) + float32(img.Pix[pos+1]) + float32(img.Pix[pos+2])
			cnt += 3
		}
	}
	gsum /= float32(256)
	if cnt > 0 {
		gsum /= float32(cnt)
	}
	return gsum
}
