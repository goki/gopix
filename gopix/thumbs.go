// Copyright (c) 2020, The gide / GoKi Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package main

import (
	"bytes"
	"fmt"
	"image"
	"log"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"time"

	"github.com/anthonynsimon/bild/clone"
	"github.com/goki/gi/gi"
	"github.com/goki/gi/girl"
	"github.com/goki/gopix/picinfo"
	"github.com/goki/ki/dirs"
	"github.com/goki/mat32"
	"github.com/goki/pi/filecat"
)

const ThumbMaxSize = 256

// DateFileFmt is the Time format for naming files by their timestamp
var DateFileFmt = "2006_01_02_15_04_05"

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
		fnext, _ := dirs.SplitExt(fn)
		pi, has := pv.AllInfo[fnext]
		if has {
			pv.Info[i] = pi
			continue
		}
		typ := filecat.SupportedFromFile(imgs[i])
		if typ.Cat() != filecat.Image { // todo: movies!
			imgs = append(imgs[:i], imgs[i+1:]...)
			pv.Info = append(pv.Info[:i], pv.Info[i+1:]...)
		} else {
			fmt.Printf("found new file: %s\n", fn)
		}
	}

	nfl = len(imgs)
	pv.PProg.Start(nfl)

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
	adir := filepath.Join(pv.ImageDir, "All")
	trdir := filepath.Join(pv.ImageDir, "Trash")
	for i := st; i < ed; i++ {
		fn := filepath.Base(imgs[i])
		fnext, _ := dirs.SplitExt(fn)
		if pv.Info[i] != nil {
			pi := pv.Info[i]
			_, err := os.Stat(pi.Thumb)
			if err == nil {
				if pv.Folder == "Trash" {
					pi.File = filepath.Join(trdir, fn)
				}
				fst, err := os.Stat(pi.File)
				if err != nil {
					log.Printf("missing file %s: err: %s\n", pi.File, err)
				} else {
					if !pi.FileMod.Before(fst.ModTime()) {
						if !pi.DateTaken.IsZero() {
							pv.PProg.ProgStep()
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
		ffn := filepath.Join(adir, fn)
		if pv.Folder == "Trash" {
			ffn = filepath.Join(trdir, fn)
		}
		pi, err := picinfo.OpenNewInfo(ffn)
		if pi == nil {
			fmt.Printf("File: %s failed Info open: err: %v\n", fn, err)
			pv.PProg.ProgStep()
			continue
		}
		num, has := pv.NumberFromFname(fnext)
		if has {
			pi.Number = num
		}
		pi.SetFileThumbFmFile(ffn, tdir)
		pv.AllMu.Lock()
		pv.AllInfo[fnext] = pi
		pv.AllMu.Unlock()
		pv.Info[i] = pi

		err = pv.ThumbGenIfNeeded(pi)
		if err != nil {
			pi.Thumb = ""
			log.Println(err)
		}
		pv.PProg.ProgStep()
	}
	pv.WaitGp.Done()
}

// NumberFromFname returns the number encoded in an img_ standard filename (no ext)
// if present.  false if not present.
func (pv *PixView) NumberFromFname(fnext string) (int, bool) {
	if !strings.HasPrefix(fnext, "img_") {
		return 0, false
	}
	nidx := strings.LastIndex(fnext, "_n")
	num, err := strconv.Atoi(fnext[nidx+2:])
	if err == nil {
		return num, true
	}
	return 0, false
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
	rgb, ok := img.(*image.RGBA)
	if !ok {
		rgb = clone.AsRGBA(img)
	}
	tr := &girl.Text{}
	rs := &girl.State{}
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
	if len(pv.AllInfo) == 0 {
		return nil
	}
	ifn := filepath.Join(pv.ImageDir, "info.json")
	os.Rename(ifn, ifn+"~")
	pv.AllMu.Lock()
	defer pv.AllMu.Unlock()
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
	imgs = imgs[1:] // first one is the directory itself

	mx := len(imgs)
	pv.PProg.Start(mx)

	bmap := make(map[string][]string) // base map of all versions
	for _, img := range imgs {
		fn := filepath.Base(img)
		fnext, _ := dirs.SplitExt(fn)
		fl, has := bmap[fnext]
		if has {
			fl = append(fl, fn)
			bmap[fnext] = fl
		} else {
			bmap[fnext] = []string{fn}
		}
		pv.PProg.ProgStep()
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

// BaseNamesMap returns a map of base file names without extensions for all files
func (pv *PixView) BaseNamesMap() map[string]string {
	bmap := make(map[string]string, len(pv.AllInfo))
	for fn := range pv.AllInfo {
		fnext, _ := dirs.SplitExt(fn)
		bmap[fnext] = fn
	}
	return bmap
}

// UniqueNameNumber returns a unique image file base name and number
// based on given datetaken, and image number
func (pv *PixView) UniqueNameNumber(dt time.Time, num int) (string, int) {
	ds := dt.Format(DateFileFmt)
	n := num
	nfn := fmt.Sprintf("img_%s_n%d", ds, n)
	for i := n; i < 100000; i++ {
		_, has := pv.AllInfo[nfn]
		if !has {
			break
		}
		n = i
		nfn = fmt.Sprintf("img_%s_n%d", ds, n)
	}
	return nfn, n
}

// RenameByDate renames image files by their date taken.
// Operates on AllInfo so must be done after that is loaded.
func (pv *PixView) RenameByDate() {
	pv.UpdateFolders()
	pv.GetFolderFiles() // greatly speeds up rename

	adir := filepath.Join(pv.ImageDir, "All")
	tdir := pv.ThumbDir()

	pv.PProg.Start(len(pv.AllInfo))
	for fn, pi := range pv.AllInfo {
		if pi.DateTaken.IsZero() {
			pv.PProg.ProgStep()
			continue
		}
		ds := pi.DateTaken.Format(DateFileFmt)
		n := pi.Number
		nfn := fmt.Sprintf("img_%s_n%d", ds, n)
		if fn == nfn {
			pv.PProg.ProgStep()
			continue
		}
		ofn := filepath.Base(pi.File)
		otf := pi.Thumb

		opi := &picinfo.Info{}
		*opi = *pi

		nfn, pi.Number = pv.UniqueNameNumber(pi.DateTaken, 0)
		pi.SetFileThumbFmBase(nfn, adir, tdir)

		nfb := filepath.Base(pi.File)

		fmt.Printf("renaming %s => %s\n", ofn, nfb)
		// fmt.Printf("rename: %s -> %s\n", fn, nfn)
		pv.RenameFile(ofn, nfb)
		os.Rename(otf, pi.Thumb)

		delete(pv.AllInfo, fn)
		pv.AllInfo[nfn] = pi
		pv.PProg.ProgStep()
	}
	fmt.Println("...Done\n")
	// gi.PromptDialog(nil, gi.DlgOpts{Title: "Done", Prompt: "Done Renaming by Date"}, gi.AddOk, gi.NoCancel, nil, nil)
	pv.DirInfo(false)
	pv.FolderFiles = nil
	return
}

// CleanAllInfo updates the AllInfo list based on actual files
func (pv *PixView) CleanAllInfo(dryRun bool) {
	adir := filepath.Join(pv.ImageDir, "All")
	pv.UpdateFolders()

	imgs, err := dirs.AllFiles(adir)
	if err != nil {
		fmt.Println(err)
		return
	}
	imgs = imgs[1:] // first one is the directory itself

	nfl := len(imgs)
	pv.PProg.Start(nfl)

	ncp := runtime.NumCPU()
	nper := nfl / ncp
	st := 0
	for i := 0; i < ncp; i++ {
		ed := st + nper
		if i == ncp-1 {
			ed = nfl
		}
		go pv.CleanAllInfoThr(dryRun, imgs, st, ed)
		pv.WaitGp.Add(1)
		st = ed
	}
	pv.WaitGp.Wait()
	for fnext, pi := range pv.AllInfo {
		if pi.Flagged {
			pi.Flagged = false
			continue
		}
		fmt.Printf("File was not found, will be deleted from list: %s\n", pi.File)
		if dryRun {
			continue
		}
		delete(pv.AllInfo, fnext)
	}
	pv.SaveAllInfo()
	fmt.Println("...Done\n")
	gi.PromptDialog(nil, gi.DlgOpts{Title: "Done", Prompt: "Done Cleaning AllInfo"}, gi.AddOk, gi.NoCancel, nil, nil)
}

func (pv *PixView) CleanAllInfoThr(dryRun bool, imgs []string, st, ed int) {
	for i := st; i < ed; i++ {
		img := imgs[i]
		typ := filecat.SupportedFromFile(img)
		if typ.Cat() != filecat.Image { // todo: movies!
			pv.PProg.ProgStep()
			continue
		}
		fn := filepath.Base(img)
		fnext, _ := dirs.SplitExt(fn)
		pv.AllMu.Lock()
		pi, has := pv.AllInfo[fnext]
		pv.AllMu.Unlock()
		if !has {
			fmt.Printf("Missing file: click on All first to ensure all files loaded! %s\n", fn)
			break
		}
		npi, err := picinfo.OpenNewInfo(pi.File)
		if err != nil {
			fmt.Printf("File: %s had error, will be moved to trash: %v\n", fn, err)
			if !dryRun {
				pv.TrashFiles(picinfo.Pics{pi})
				os.Remove(pi.Thumb)
			}
			pv.PProg.ProgStep()
			continue
		}
		num, has := pv.NumberFromFname(fnext)
		if has {
			npi.Number = num
		}
		dl := pi.DiffsTo(npi)
		if len(dl) > 0 {
			fmt.Printf("\nFile: %s had following diffs (n = %d)\n", fn, len(dl))
			for _, d := range dl {
				fmt.Printf(d)
			}
		}

		pi.Flagged = true // mark as good
	}
	pv.WaitGp.Done()
}

// CleanDupes checks for duplicate files based on file sizes
func (pv *PixView) CleanDupes(dryRun bool) {
	// adir := filepath.Join(pv.ImageDir, "All")
	pv.UpdateFolders()

	smap := make(map[int64]picinfo.Pics, len(pv.AllInfo))

	smax := int64(0)
	for _, pi := range pv.AllInfo {
		fi, err := os.Stat(pi.Thumb)
		if err != nil {
			continue
		}
		sz := fi.Size()
		if sz > smax {
			smax = sz
		}
		pis, has := smap[sz]
		if has {
			pis = append(pis, pi)
			smap[sz] = pis
		} else {
			smap[sz] = picinfo.Pics{pi}
		}
	}

	mx := len(smap)
	pv.PProg.Start(mx)

	szs := make([]int64, mx)
	idx := 0
	for sz := range smap {
		szs[idx] = sz
		idx++
	}

	ncp := runtime.NumCPU()
	nper := mx / ncp
	st := 0
	for i := 0; i < ncp; i++ {
		ed := st + nper
		if i == ncp-1 {
			ed = mx
		}
		go pv.CleanDupesThr(dryRun, smax, szs, smap, st, ed)
		pv.WaitGp.Add(1)
		st = ed
	}
	pv.WaitGp.Wait()
	pv.SaveAllInfo()
	fmt.Println("...Done\n")
	gi.PromptDialog(nil, gi.DlgOpts{Title: "Done", Prompt: "Done Cleaning Duplicates"}, gi.AddOk, gi.NoCancel, nil, nil)
	pv.DirInfo(false)
}

func (pv *PixView) CleanDupesThr(dryRun bool, smax int64, szs []int64, smap map[int64]picinfo.Pics, st, ed int) {
	b1 := bytes.NewBuffer(make([]byte, 0, smax))
	b2 := bytes.NewBuffer(make([]byte, 0, smax))
	for si := st; si < ed; si++ {
		sz := szs[si]
		pis := smap[sz]
		if len(pis) <= 1 {
			pv.PProg.ProgStep()
			continue
		}
		npi := len(pis)
		did := false
		for i, pi := range pis {
			f1, err := os.Open(pi.File)
			if err != nil {
				log.Println(err)
				continue
			}
			b1.Reset()
			_, err = b1.ReadFrom(f1)
			if err != nil {
				f1.Close()
				log.Println(err)
				continue
			}
			f1.Close()

			for j := i + 1; j < npi; j++ {
				opi := pis[j]
				f2, err := os.Open(opi.File)
				if err != nil {
					log.Println(err)
					continue
				}
				b2.Reset()
				_, err = b2.ReadFrom(f2)
				if err != nil {
					f2.Close()
					log.Println(err)
					continue
				}
				f2.Close()
				if bytes.Equal(b1.Bytes(), b2.Bytes()) {
					fmt.Printf("duplicates: %s == %s\n", filepath.Base(pi.File), filepath.Base(opi.File))
					did = true
					if !dryRun {
						if pi.Number < opi.Number {
							pv.TrashFiles(picinfo.Pics{opi})
						} else if pi.Number > opi.Number {
							pv.TrashFiles(picinfo.Pics{pi})
						} else {
							pv.TrashFiles(picinfo.Pics{opi})
						}
					}
				}
			}
			if did {
				break
			}
		}
		pv.PProg.ProgStep()
	}
	pv.WaitGp.Done()
}
