// Copyright (c) 2020, The gide / GoKi Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package main

import (
	"fmt"
	"image"
	"log"
	"os"
	"path/filepath"
	"runtime"
	"strings"

	"github.com/goki/gi/gi"
	"github.com/goki/gopix/picinfo"
	"github.com/goki/ki/dirs"
	"github.com/goki/mat32"
	"github.com/goki/pi/filecat"
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

// DirInfo updates Info and thumbnails based on current folder.
// If reset, reset selections (e.g., when going to a new folder)
func (pv *PixView) DirInfo(reset bool) {
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
	pv.Info = make(picinfo.Pics, nfl)

	// fmt.Printf("N files %d\n", nfl)

	// first pass fill in from existing info -- no locking
	for i := nfl - 1; i >= 0; i-- {
		fn := filepath.Base(imgs[i])
		pi, has := pv.AllInfo[fn]
		if has {
			pv.Info[i] = pi
			continue
		}
		typ := filecat.SupportedFromFile(fn)
		if typ.Cat() != filecat.Image { // todo: movies!
			imgs = append(imgs[:i], imgs[i+1:]...)
			pv.Info = append(pv.Info[:i], pv.Info[i+1:]...)
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
	ig.SetImages(pv.Thumbs, reset)
	// fmt.Printf("done\n")
}

func (pv *PixView) InfoUpdtThr(fdir string, imgs []string, st, ed int) {
	tdir := pv.ThumbDir()
	for i := st; i < ed; i++ {
		if pv.Info[i] != nil {
			pi := pv.Info[i]
			_, err := os.Stat(pi.Thumb)
			if err == nil {
				fst, err := os.Stat(pi.File)
				if err != nil {
					log.Printf("missing file %s: err: %s\n", pi.File, err)
				} else {
					if fst.ModTime() == pi.FileMod {
						if !pi.DateTaken.IsZero() {
							continue
						}
						fmt.Printf("redoing thumb to update date taken: %v\n", pi.File)
						os.Remove(pi.Thumb)
					}
					// fmt.Printf("Image file updated: %s\n", pi.File)
				}
			}
			pv.Info[i] = nil // regen
		}
		fn := filepath.Base(imgs[i])
		ffn := filepath.Join(fdir, fn)
		pi, err := picinfo.OpenExif(ffn)
		if pi == nil {
			fmt.Printf("failed exif: %v err: %v\n", fn, err)
			continue
		}
		fnext, _ := dirs.SplitExt(fn)
		tfn := filepath.Join(tdir, fnext+".jpg")
		pi.Thumb = tfn

		pv.AllMu.Lock()
		pv.AllInfo[fn] = pi
		pv.AllMu.Unlock()
		pv.Info[i] = pi

		err = pv.ThumbGenIfNeeded(pi)
		if err != nil {
			pi.Thumb = ""
			log.Println(err)
		}
	}
	pv.WaitGp.Done()
}

// ThumbGenIfNeeded generates a thumb file for given image file (picinfo.Info)
// if the image file modification date is newer than the thumb image file date,
// or thumb file does not exist.
func (pv *PixView) ThumbGenIfNeeded(pi *picinfo.Info) error {
	tst, err := os.Stat(pi.Thumb)
	if err != nil {
		return pv.ThumbGen(pi)
	}
	if tst.ModTime().Before(pi.FileMod) {
		return pv.ThumbGen(pi)
	}
	return nil
}

// ThumbGen generates a thumb file for given image file (picinfo.Info)
// and saves it in the Thumb file.
func (pv *PixView) ThumbGen(pi *picinfo.Info) error {
	img, err := picinfo.OpenImage(pi.File)
	if err != nil {
		return err
	}
	img = gi.ImageResizeMax(img, ThumbMaxSize)
	img = picinfo.OrientImage(img, pi.Orient)
	isz := img.Bounds().Size()
	rgb := img.(*image.RGBA)
	tr := &gi.TextRender{}
	rs := &gi.RenderState{}
	rs.Init(isz.X, isz.Y, rgb)
	rs.Bounds.Max = isz
	ds := pi.DateTaken.Format("2006:01:02")
	avg := AvgImgGrey(rgb, image.Rect(5, 5, 100, 25))
	if avg < .5 {
		pv.Sty.Font.Color.SetUInt8(0xff, 0xff, 0xff, 0xff)
	} else {
		pv.Sty.Font.Color.SetUInt8(0, 0, 0, 0xff)
	}
	tr.SetString(ds, &pv.Sty.Font, &pv.Sty.UnContext, &pv.Sty.Text, true, 0, 1)
	tr.RenderTopPos(rs, mat32.Vec2{5, 5})
	err = picinfo.SaveImage(pi.Thumb, rgb)
	return err
}

// OpenAllInfo open cached info on all pictures
func (pv *PixView) OpenAllInfo() error {
	fmt.Printf("Loading All photos info\n")
	ifn := filepath.Join(pv.ImageDir, "info.json")
	err := pv.AllInfo.OpenJSON(ifn)
	adir := filepath.Join(pv.ImageDir, "All")
	tdir := pv.ThumbDir()
	pv.AllInfo.SetFileThumb(adir, tdir)
	fmt.Printf("%d Pictures Loaded\n", len(pv.AllInfo))
	return err
}

// SaveAllInfo save cached info on all pictures
func (pv *PixView) SaveAllInfo() error {
	pv.AllMu.Lock()
	defer pv.AllMu.Unlock()
	ifn := filepath.Join(pv.ImageDir, "info.json")
	err := pv.AllInfo.SaveJSON(ifn)
	return err
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

// UniquifyBaseNames ensures that the base names (pre image extension)
// of All files are unique -- we use common .jpg extension for thumbs,
// so this must be true
func (pv *PixView) UniquifyBaseNames() {
	// fmt.Printf("Ensuring base names are unique...\n")
	adir := filepath.Join(pv.ImageDir, "All")

	pv.UpdateFolders()
	pv.GetFolderFiles() // greatly speeds up rename

	imgs, err := dirs.AllFiles(adir)
	if err != nil {
		fmt.Println(err)
		return
	}
	imgs = imgs[1:]                   // first one is the directory itself
	bmap := make(map[string][]string) // base map of all versions
	for _, img := range imgs {
		fn := filepath.Base(img)
		ext := filepath.Ext(fn)
		b := strings.TrimSuffix(fn, ext)
		fl, has := bmap[b]
		if has {
			fl = append(fl, fn)
			bmap[b] = fl
		} else {
			bmap[b] = []string{fn}
		}
	}

	rmap := make(map[string]string) // rename map

	for b, fl := range bmap {
		nf := len(fl)
		if nf == 1 {
			continue
		}
		fi := 0
		for i := 1; i < 100000; i++ {
			tb := fmt.Sprintf("%s_%d", b, i)
			if _, has := bmap[tb]; !has {
				fn := fl[fi]
				ext := filepath.Ext(fn)
				rmap[fn] = tb + ext
				fi++
				if fi >= nf {
					break
				}
			}
		}
	}

	// fmt.Printf("Renaming %d files...\n", len(rmap))
	for of, rf := range rmap {
		fmt.Printf("%s -> %s\n", of, rf)
		pv.RenameFile(of, rf)
	}

	pv.FolderFiles = nil // done
}

// RenameByDate renames image files by their date taken.
// Operates on AllInfo so must be done after that is loaded.
func (pv *PixView) RenameByDate() {
	fmt.Printf("Renaming files by their DateTaken...\n")

	pv.UpdateFolders()
	pv.GetFolderFiles() // greatly speeds up rename

	adir := filepath.Join(pv.ImageDir, "All")
	tdir := pv.ThumbDir()

	bmap := make(map[string]string, len(pv.AllInfo))
	for fn := range pv.AllInfo {
		fnext, _ := dirs.SplitExt(fn)
		bmap[fnext] = fn
	}

	for fn, pi := range pv.AllInfo {
		if pi.DateTaken.IsZero() {
			continue
		}
		fnext, ext := dirs.SplitExt(fn)
		ds := pi.DateTaken.Format("2006_01_02_15_04_05")
		lext := strings.ToLower(ext)
		n := pi.Number
		nfnb := fmt.Sprintf("img_%s_n%d", ds, n)
		nfn := nfnb + lext

		if fn == nfn {
			continue
		}

		for i := n; i < 100000; i++ {
			_, hasa := pv.AllInfo[nfn]
			_, hasb := bmap[nfnb]
			if !hasa && !hasb {
				break
			}
			n = i
			nfnb = fmt.Sprintf("img_%s_n%d", ds, n)
			nfn = nfnb + lext
		}
		pi.Number = n

		// fmt.Printf("rename: %s -> %s\n", fn, nfn)
		pv.RenameFile(fn, nfn)
		pi.File = filepath.Join(adir, nfn)

		delete(bmap, fnext)
		bmap[nfnb] = nfn

		tfn := nfnb + ".jpg"
		tnfn := filepath.Join(tdir, tfn)
		os.Rename(pi.Thumb, tnfn)
		pi.Thumb = tnfn

		delete(pv.AllInfo, fn)
		pv.AllInfo[nfn] = pi
	}
	fmt.Println("...Done\n")
	gi.PromptDialog(nil, gi.DlgOpts{Title: "Done", Prompt: "Done Renaming by Date"}, gi.AddOk, gi.NoCancel, nil, nil)
	pv.DirInfo(false)
	return
}
