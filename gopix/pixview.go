// Copyright (c) 2020, The GoKi Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package main

import (
	"fmt"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/anthonynsimon/bild/transform"
	"github.com/goki/gi/gi"
	"github.com/goki/gi/giv"
	"github.com/goki/gi/oswin"
	"github.com/goki/gopix/imgrid"
	"github.com/goki/gopix/imgview"
	"github.com/goki/gopix/picinfo"
	"github.com/goki/ki/dirs"
	"github.com/goki/ki/ki"
	"github.com/goki/ki/kit"
	"github.com/goki/ki/sliceclone"
	"github.com/goki/pi/filecat"
)

// PixView is a picture viewer with a folder view on the left, and a tab bar with image grid
// and currently selected view.
type PixView struct {
	gi.Frame
	CurFile     string                `desc:"current file -- viewed in Current bitmap or last selected in ImgGrid"`
	ImageDir    string                `desc:"directory with the images"`
	Folder      string                `desc:"current folder"`
	Folders     []string              `desc:"list of all folders, excluding All and Trash"`
	FolderFiles []map[string]struct{} `desc:"list of all files in all Folders -- used for e.g., large renames"`
	Files       giv.FileTree          `desc:"all the files in the project directory and subdirectories"`
	Info        picinfo.Pics          `desc:"info for all the pictures in current folder"`
	AllInfo     picinfo.PicMap        `desc:"map of info for all files"`
	AllMu       sync.Mutex            `desc:"mutex protecting AllInfo"`
	Thumbs      []string              `view:"-" desc:"desc list of all thumb files in current folder -- sent to ImgGrid -- must be in 1-to-1 order with Info"`
	WaitGp      sync.WaitGroup        `view:"-" desc:"wait group for synchronizing threaded layer calls"`
}

var KiT_PixView = kit.Types.AddType(&PixView{}, PixViewProps)

// AddNewPixView adds a new pixview to given parent node, with given name.
func AddNewPixView(parent ki.Ki, name string) *PixView {
	return parent.AddNewChild(KiT_PixView, name).(*PixView)
}

// UpdateFiles updates the list of files saved in project
func (pv *PixView) UpdateFiles() {
	pv.UpdateFolders()
	pv.Files.OpenPath(pv.ImageDir)
	ft := pv.FileTreeView()
	ft.SetFullReRender()
	ft.UpdateSig()
}

// UpdateFolders updates the list of current folders
func (pv *PixView) UpdateFolders() {
	pv.Folders = dirs.Dirs(pv.ImageDir)
	nf := len(pv.Folders)
	for i := nf - 1; i >= 0; i-- {
		f := pv.Folders[i]
		if f == "All" || f == "Trash" {
			pv.Folders = append(pv.Folders[:i], pv.Folders[i+1:]...)
		}
	}
}

// GetFolderFiles gets a list of files for each folder
// do this for operations that require this info
func (pv *PixView) GetFolderFiles() {
	pv.FolderFiles = make([]map[string]struct{}, len(pv.Folders))
	for i, f := range pv.Folders {
		fdir := filepath.Join(pv.ImageDir, f)
		imgs, err := dirs.AllFiles(fdir)
		if err != nil {
			log.Println(err)
			continue
		}
		imgs = imgs[1:]
		fmap := make(map[string]struct{}, len(imgs))
		for _, img := range imgs {
			fn := filepath.Base(img)
			fmap[fn] = struct{}{}
		}
		pv.FolderFiles[i] = fmap
	}
}

// Config configures the widget, with images at given path
func (pv *PixView) Config(imgdir string) {
	if pv.HasChildren() {
		return
	}
	updt := pv.UpdateStart()
	defer pv.UpdateEnd(updt)

	pv.ImageDir = imgdir
	pv.Folder = "All"

	// do all file-level updating now
	pv.UpdateFolders()
	pv.UniquifyBaseNames()
	pv.OpenAllInfo()

	pv.Lay = gi.LayoutVert
	pv.SetProp("spacing", gi.StdDialogVSpaceUnits)
	gi.AddNewToolBar(pv, "toolbar")
	split := gi.AddNewSplitView(pv, "splitview")

	ftfr := gi.AddNewFrame(split, "filetree", gi.LayoutVert)
	ft := ftfr.AddNewChild(KiT_FileTreeView, "filetree").(*FileTreeView)
	ft.OpenDepth = 4

	tv := gi.AddNewTabView(split, "tabs")
	tv.NoDeleteTabs = true

	ig := tv.AddNewTab(imgrid.KiT_ImgGrid, "Images").(*imgrid.ImgGrid)
	ig.ImageMax = ThumbMaxSize
	ig.Config(true)
	ig.CtxtMenuFunc = pv.ImgGridCtxtMenu

	pic := tv.AddNewTab(imgview.KiT_ImgView, "Current").(*imgview.ImgView)
	pic.SetStretchMax()

	split.SetSplits(.1, .9)

	pv.UpdateFiles()
	ft.SetRootNode(&pv.Files)

	pv.ConfigToolbar()

	ft.TreeViewSig.Connect(pv.This(), func(recv, send ki.Ki, sig int64, data interface{}) {
		if data == nil {
			return
		}
		tvn, _ := data.(ki.Ki).Embed(KiT_FileTreeView).(*FileTreeView)
		pvv, _ := recv.Embed(KiT_PixView).(*PixView)
		if tvn.SrcNode != nil {
			fn := tvn.SrcNode.Embed(giv.KiT_FileNode).(*giv.FileNode)
			switch sig {
			case int64(giv.TreeViewSelected):
				pvv.FileNodeSelected(fn, tvn)
			case int64(giv.TreeViewOpened):
				pvv.FileNodeOpened(fn, tvn)
			case int64(giv.TreeViewClosed):
				pvv.FileNodeClosed(fn, tvn)
			}
		}
	})
	ig.WidgetSig.Connect(pv.This(), func(recv, send ki.Ki, sig int64, data interface{}) {
		if sig == int64(gi.WidgetSelected) {
			pvv, _ := recv.Embed(KiT_PixView).(*PixView)
			idx := data.(int)
			if idx >= 0 && idx < len(pv.Info) {
				fn := filepath.Base(pv.Info[idx].File)
				pvv.CurFile = fn
			}
		}
	})
	ig.ImageSig.Connect(pv.This(), func(recv, send ki.Ki, sig int64, data interface{}) {
		igg, _ := send.Embed(imgrid.KiT_ImgGrid).(*imgrid.ImgGrid)
		pvv, _ := recv.Embed(KiT_PixView).(*PixView)
		idx := data.(int)
		if idx < 0 || idx >= len(pv.Info) {
			return
		}
		pi := pv.Info[idx]
		fn := filepath.Base(pi.File)
		switch imgrid.ImgGridSignals(sig) {
		case imgrid.ImgGridDeleted:
			if pvv.Folder == "All" {
				pvv.TrashFiles([]string{fn})
			} else {
				pvv.DeleteInFolder(pvv.Folder, []string{fn}) // this works for Trash too -- permanent..
			}
			pvv.PicDeleteAt(idx)
		case imgrid.ImgGridInserted:
			if pvv.Folder == "Trash" {
				igg.Images = sliceclone.String(pv.Thumbs)
				return
			}
			pvv.ImgGridMoveDates(idx)
		case imgrid.ImgGridDoubleClicked:
			pvv.ViewFile(pi)
		}
	})
}

// SplitView returns the main SplitView
func (pv *PixView) SplitView() *gi.SplitView {
	return pv.ChildByName("splitview", 1).(*gi.SplitView)
}

// FileTreeView returns the main FileTreeView
func (pv *PixView) FileTreeView() *FileTreeView {
	return pv.SplitView().Child(0).Child(0).(*FileTreeView)
}

// Tabs returns the TabView
func (pv *PixView) Tabs() *gi.TabView {
	return pv.SplitView().Child(1).(*gi.TabView)
}

// ImgGrid returns the ImgGrid
func (pv *PixView) ImgGrid() *imgrid.ImgGrid {
	return pv.Tabs().TabByName("Images").(*imgrid.ImgGrid)
}

// CurImgView returns the ImgView for viewing the current file
func (pv *PixView) CurImgView() *imgview.ImgView {
	return pv.Tabs().TabByName("Current").(*imgview.ImgView)
}

// ToolBar returns the toolbar widget
func (pv *PixView) ToolBar() *gi.ToolBar {
	return pv.ChildByName("toolbar", 0).(*gi.ToolBar)
}

// PicDeleteAt deletes image at given index
func (pv *PixView) PicDeleteAt(idx int) {
	pv.Info = append(pv.Info[:idx], pv.Info[idx+1:]...)
	pv.Thumbs = append(pv.Thumbs[:idx], pv.Thumbs[idx+1:]...)
}

// PicIdxByName returns the index in current Info of given file
func (pv *PixView) PicIdxByName(fname string) int {
	for i, pi := range pv.Info {
		if pi.File == fname {
			return i
		}
	}
	return -1
}

// PicIdxByThumb returns the index in current Info of given thumb name
func (pv *PixView) PicIdxByThumb(tname string) int {
	for i, pi := range pv.Info {
		if pi.Thumb == tname {
			return i
		}
	}
	return -1
}

// PicInsertAt inserts image(s) at given index
// func (pv *PixView) PicInsertAt(idx int, files []string) {
// 	ni := len(files)
//
// 	// nt := append(pv.Info, files...) // first append to end
// 	// copy(nt[idx+ni:], nt[idx:])     // move stuff to end
// 	// copy(nt[idx:], files)           // copy into position
// 	// pv.Info = nt
// 	nt := append(pv.Thumbs, files...) // first append to end
// 	copy(nt[idx+ni:], nt[idx:])       // move stuff to end
// 	copy(nt[idx:], files)             // copy into position
// 	pv.Thumbs = nt
// }

// FileNodeSelected is called whenever tree browser has file node selected
func (pv *PixView) FileNodeSelected(fn *giv.FileNode, tvn *FileTreeView) {
	if fn.IsDir() {
		pv.Folder = fn.Nm
		pv.Tabs().SelectTabByName("Images")
		pv.DirInfo(true) // reset
	}
}

// FileNodeOpened is called whenever file node is double-clicked in file tree
func (pv *PixView) FileNodeOpened(fn *giv.FileNode, tvn *FileTreeView) {
	switch fn.Info.Cat {
	case filecat.Folder:
		if !fn.IsOpen() && fn.Nm != "All" { // all is too big!
			tvn.SetOpen()
			fn.OpenDir()
		}
	case filecat.Video:
		fallthrough
	case filecat.Audio:
		fn.OpenFileDefault()
	case filecat.Sheet:
		fn.OpenFileDefault()
	case filecat.Bin:
		fn.OpenFileDefault()
	case filecat.Archive:
		fn.OpenFileDefault()
	case filecat.Image:
		fn.OpenFileDefault()
	default:
		fn.OpenFileDefault()
	}
}

// FileNodeClosed is called whenever file tree browser node is closed
func (pv *PixView) FileNodeClosed(fn *giv.FileNode, tvn *FileTreeView) {
	if fn.IsDir() {
		if fn.IsOpen() {
			fn.CloseDir()
		}
	}
}

// ConfigToolbar adds a PixView toolbar.
func (pv *PixView) ConfigToolbar() {
	tb := pv.ToolBar()
	if tb != nil && tb.HasChildren() {
		return
	}
	tb.SetStretchMaxWidth()
	giv.ToolBarView(pv, pv.Viewport, tb)
}

func (pv *PixView) Render2D() {
	pv.ToolBar().UpdateActions()
	if win := pv.ParentWindow(); win != nil {
		if !win.IsResizing() {
			win.MainMenuUpdateActives()
		}
	}
	pv.Frame.Render2D()
}

//////////////////////////////////////////////////////////////////////////////////
//  Image grid actions

func (pv *PixView) ImgGridCtxtMenu(m *gi.Menu, idx int) {
	ig := pv.ImgGrid()
	nf := len(pv.Info)
	if idx >= nf-1 || idx < 0 {
		return
	}
	pi := pv.Info[idx]
	m.AddAction(gi.ActOpts{Label: "Info", Data: idx},
		pv.This(), func(recv, send ki.Ki, sig int64, data interface{}) {
			pv.InfoFile(pi)
		})
	m.AddAction(gi.ActOpts{Label: "SetDate", Data: idx},
		pv.This(), func(recv, send ki.Ki, sig int64, data interface{}) {
			pv.CurFile = filepath.Base(pi.File)
			giv.CallMethod(pv, "SetDateTakenCur", pv.Viewport)
		})
	m.AddSeparator("clip")
	m.AddAction(gi.ActOpts{Label: "Copy", Data: idx},
		pv.This(), func(recv, send ki.Ki, sig int64, data interface{}) {
			ig.CopyIdxs(true)
		})
	m.AddAction(gi.ActOpts{Label: "Cut", Data: idx},
		pv.This(), func(recv, send ki.Ki, sig int64, data interface{}) {
			ig.CutIdxs()
		})
	m.AddAction(gi.ActOpts{Label: "Paste", Data: idx},
		pv.This(), func(recv, send ki.Ki, sig int64, data interface{}) {
			ig.PasteIdx(data.(int))
		})
	m.AddAction(gi.ActOpts{Label: "Duplicate", Data: idx},
		pv.This(), func(recv, send ki.Ki, sig int64, data interface{}) {
			pv.Duplicate(pi)
		})
	m.AddAction(gi.ActOpts{Label: "Delete", Data: idx},
		pv.This(), func(recv, send ki.Ki, sig int64, data interface{}) {
			ig.CutIdxs()
		})
}

//////////////////////////////////////////////////////////////////////////////////
//  file functions

// LinkToFolder creates links in given folder o given files in ../all
func (pv *PixView) LinkToFolder(fnm string, files []string) {
	tdir := filepath.Join(pv.ImageDir, fnm)
	for _, f := range files {
		lf := filepath.Join(tdir, f)
		sf := filepath.Join("../All", f)
		err := os.Symlink(sf, lf)
		if err != nil {
			log.Println(err)
		}
	}
}

// RenameFile renames file in All and any folders where it might be linked
// input is just the file name, no path
func (pv *PixView) RenameFile(oldnm, newnm string) {
	adir := filepath.Join(pv.ImageDir, "All")
	aofn := filepath.Join(adir, oldnm)
	anfn := filepath.Join(adir, newnm)
	os.Rename(aofn, anfn)

	sf := filepath.Join("../All", newnm)
	for i, f := range pv.Folders {
		fdir := filepath.Join(pv.ImageDir, f)
		rename := false
		if pv.FolderFiles != nil {
			fmap := pv.FolderFiles[i]
			_, rename = fmap[oldnm]
		} else {
			imgs, err := dirs.AllFiles(fdir)
			if err != nil {
				continue
			}
			imgs = imgs[1:]
			for _, img := range imgs {
				fn := filepath.Base(img)
				if fn == oldnm {
					rename = true
					break
				}
			}
		}
		if rename {
			err := os.Remove(filepath.Join(fdir, oldnm))
			if err != nil {
				log.Println(err)
			}
			err = os.Symlink(sf, filepath.Join(fdir, newnm))
			if err != nil {
				log.Println(err)
			}
		}
	}
}

// DeleteInFolder deletes links in folder
func (pv *PixView) DeleteInFolder(fnm string, files []string) {
	tdir := filepath.Join(pv.ImageDir, fnm)
	for _, f := range files {
		lf := filepath.Join(tdir, f)
		err := os.Remove(lf)
		if err != nil {
			log.Println(err)
		}
	}
}

// TrashFiles moves given files from All to Trash, and removes symlinks from
// any folders
func (pv *PixView) TrashFiles(files []string) {
	adir := filepath.Join(pv.ImageDir, "All")
	tdir := filepath.Join(pv.ImageDir, "Trash")
	os.MkdirAll(tdir, 0775)
	fmap := make(map[string]struct{}, len(files))
	for _, f := range files {
		fmap[f] = struct{}{}
		tfn := filepath.Join(tdir, f)
		afn := filepath.Join(adir, f)
		err := os.Rename(afn, tfn)
		if err != nil {
			log.Println(err)
		}
	}
	drs := dirs.Dirs(pv.ImageDir)
	for _, d := range drs {
		if d == "All" || d == "Trash" {
			continue
		}
		dp := filepath.Join(pv.ImageDir, d)
		dfs, _ := dirs.AllFiles(dp)
		for _, f := range dfs {
			fb := filepath.Base(f)
			if _, has := fmap[fb]; has {
				os.Remove(f)
			}
		}
	}
}

// UntrashFiles moves given files from Trash to All (recover from trash)
func (pv *PixView) UntrashFiles(files []string) {
	adir := filepath.Join(pv.ImageDir, "All")
	tdir := filepath.Join(pv.ImageDir, "Trash")
	os.MkdirAll(tdir, 0775)
	for _, f := range files {
		tfn := filepath.Join(tdir, f)
		afn := filepath.Join(adir, f)
		err := os.Rename(tfn, afn)
		if err != nil {
			log.Println(err)
		}
	}
}

// ViewFile views given file in the full-size bitmap view
func (pv *PixView) ViewFile(pi *picinfo.Info) {
	pv.Tabs().SelectTabByName("Current")
	pv.CurFile = filepath.Base(pi.File)
	iv := pv.CurImgView()
	iv.SetInfo(pi)
}

// Duplicate duplicates image
func (pv *PixView) Duplicate(pi *picinfo.Info) error {
	bmap := pv.BaseNamesMap()
	fn := filepath.Base(pi.File)
	fnext, ext := dirs.SplitExt(fn)
	lext := strings.ToLower(ext)
	nfn, n := pv.UniqueNameNumber(pi.DateTaken, pi.Number, lext, bmap)
	npi := &picinfo.Info{}
	*npi = *pi
	adir := filepath.Join(pv.ImageDir, "All")
	npi.File = filepath.Join(adir, nfn)
	npi.Number = n
	giv.CopyFile(npi.File, pi.File, 0664)
	npi.UpdateFileMod()
	fnext, _ = dirs.SplitExt(nfn)
	tdir := pv.ThumbDir()
	tfn := filepath.Join(tdir, fnext+".jpg")
	npi.Thumb = tfn
	giv.CopyFile(npi.Thumb, pi.Thumb, 0664)
	pv.AllInfo[nfn] = npi
	if pv.Folder != "All" {
		pv.LinkToFolder(pv.Folder, []string{nfn})
	}
	pv.DirInfo(false)
	return nil
}

// CheckCur checks that current file name is set and exists in AllInfo
// returns nil if not, and opens a prompt dialog
func (pv *PixView) CheckCur() *picinfo.Info {
	pi, has := pv.AllInfo[pv.CurFile]
	if !has {
		gi.PromptDialog(nil, gi.DlgOpts{Title: "No Current File", Prompt: "Please select a valid image file and retry"}, gi.AddOk, gi.NoCancel, nil, nil)
		return nil
	}
	return pi
}

// CheckSel checks for and returns selected files from img grid.
// Prompts if none and no CurFile either (and returns nil in this case)
func (pv *PixView) CheckSel() []*picinfo.Info {
	ig := pv.ImgGrid()
	si := ig.SelectedIdxsList(false)
	n := len(si)
	if n == 0 {
		pi := pv.CheckCur()
		if pi == nil {
			return nil
		}
		return []*picinfo.Info{pi}
	}
	pis := make([]*picinfo.Info, n)
	for i, fi := range si {
		pis[i] = pv.Info[fi]
	}
	return pis
}

// OpenCurDefault opens the current file using the OS default open command
func (pv *PixView) OpenCurDefault() {
	pi := pv.CheckCur()
	if pi == nil {
		return
	}
	pv.OpenDefault(pi)
}

// OpenDefault opens the given file using the OS default open command
func (pv *PixView) OpenDefault(pi *picinfo.Info) {
	cstr := giv.OSOpenCommand()
	cmd := exec.Command(cstr, pi.File)
	out, _ := cmd.CombinedOutput()
	fmt.Printf("%s\n", out)
}

// OpenCurGimp opens the current file using gimp
func (pv *PixView) OpenCurGimp() {
	pi := pv.CheckCur()
	if pi == nil {
		return
	}
	pv.OpenGimp(pi)
}

// OpenGimp opens the given file using gimp
func (pv *PixView) OpenGimp(pi *picinfo.Info) {
	var cmd *exec.Cmd
	if oswin.TheApp.Platform() == oswin.MacOS {
		cmd = exec.Command("open", "-a", "Gimp", pi.File)
	} else {
		cmd = exec.Command("gimp", pi.File)
	}
	out, _ := cmd.CombinedOutput()
	fmt.Printf("%s\n", out)
}

// InfoCur shows metadata info about current file
func (pv *PixView) InfoCur() {
	pi := pv.CheckCur()
	if pi == nil {
		return
	}
	pv.InfoFile(pi)
}

// InfoFile shows metadata info about current file
func (pv *PixView) InfoFile(pi *picinfo.Info) {
	giv.StructViewDialog(pv.Viewport, pi, giv.DlgOpts{Title: "Picture Info: " + pi.File}, nil, nil)
}

// MapCur shows GPS coordinates for current file on google maps
func (pv *PixView) MapCur() {
	pi := pv.CheckCur()
	if pi == nil {
		return
	}
	pv.MapFile(pi)
}

// MapFile shows GPS coordinates for file on google maps
func (pv *PixView) MapFile(pi *picinfo.Info) {
	if pi.GPSLoc.Lat == 0 && pi.GPSLoc.Long == 0 {
		gi.PromptDialog(nil, gi.DlgOpts{Title: "No GPS Location", Prompt: "That file does not have a GPS location"}, gi.AddOk, gi.NoCancel, nil, nil)
		return
	}
	url := fmt.Sprintf("https://www.google.com/maps/search/?api=1&query=%g,%g", pi.GPSLoc.Lat, pi.GPSLoc.Long)
	oswin.TheApp.OpenURL(url)
}

// SaveExifSel saves updated Exif information for currently selected files.
// This will change file type if it is not already a Jpeg as that is only supported type.
func (pv *PixView) SaveExifSel() {
	pis := pv.CheckSel()
	n := len(pis)
	if n == 0 {
		return
	}
	for _, pi := range pis {
		pv.SaveExifFile(pi)
	}
	pv.FolderFiles = nil
	pv.DirInfo(false) // update -- also saves updated info
}

// RenameAsJpeg renames given file as a Jpeg file instead of whatever it was originally.
// This calls GetFolderFiles() if FolderFiles is empty -- can reset that to nil in an outer loop.
// Info record will have the new name after.
func (pv *PixView) RenameAsJpeg(pi *picinfo.Info) {
	if pv.FolderFiles == nil {
		pv.GetFolderFiles()
	}
	fnb := filepath.Base(pi.File)
	fnext, _ := dirs.SplitExt(fnb)
	nfn := fnext + ".jpg"
	pv.RenameFile(fnb, nfn)
	adir := filepath.Join(pv.ImageDir, "All")
	pi.File = filepath.Join(adir, nfn)
}

// SaveExifFile saves updated Exif information for given file.
// This will change file type if it is not already a Jpeg as that is only supported type.
// This calls GetFolderFiles() if FolderFiles is empty -- can reset that to nil in an outer loop
func (pv *PixView) SaveExifFile(pi *picinfo.Info) error {
	if pi.Sup != filecat.Jpeg {
		fmt.Printf("Note: changing file to Jpeg instead of %s\n", pi.Sup.String())
		img, err := picinfo.OpenImage(pi.File)
		if err != nil {
			log.Println(err)
			return err
		}
		pv.RenameAsJpeg(pi)
		pi.Size = img.Bounds().Size()
		err = pi.SaveJpegNew(img)
		pv.ThumbGen(pi)
		return err
	}
	err := pi.SaveJpegUpdated()
	pv.ThumbGen(pi)
	return err
}

// RotateLeftSel rotates selected images left 90
func (pv *PixView) RotateLeftSel() {
	pv.RotateSel(-90)
}

// RotateRightSel rotates selected images right 90
func (pv *PixView) RotateRightSel() {
	pv.RotateSel(90)
}

// RotateSel rotates selected images by given number of degrees (+ = right, - = left).
// +/- 90 and 180 are special cases:
// If a Jpeg file, rotation is done through the Orientation Exif
// setting, otherwise it is manually rotated and saved, except if it is an Heic file
// which must be converted to jpeg at this point..
func (pv *PixView) RotateSel(deg float32) {
	pis := pv.CheckSel()
	n := len(pis)
	if n == 0 {
		return
	}
	for _, pi := range pis {
		pv.RotateImage(pi, deg)
	}
	pv.FolderFiles = nil
	pv.DirInfo(false) // update -- also saves updated info
}

// RotateImage rotates image by given number of degrees (+ = right, - = left).
// +/- 90 and 180 are special cases:
// If a Jpeg file, rotation is done through the Orientation Exif
// setting, otherwise it is manually rotated and saved, except if it is an Heic file
// which must be converted to jpeg at this point..
func (pv *PixView) RotateImage(pi *picinfo.Info, deg float32) error {
	non90 := deg != 90 && deg != -90 && deg != 180
	if non90 || pi.Sup != filecat.Jpeg {
		img, err := picinfo.OpenImage(pi.File)
		if err != nil {
			log.Println(err)
			return err
		}
		img = picinfo.OrientImage(img, pi.Orient)
		opts := &transform.RotationOptions{ResizeBounds: true}
		img = transform.Rotate(img, float64(deg), opts)
		switch pi.Sup {
		case filecat.Jpeg:
			rawExif, _ := picinfo.OpenRawExif(pi.File)
			pi.SaveJpegUpdatedExif(rawExif, img)
		case filecat.Heic:
			rawExif, _ := picinfo.OpenRawExif(pi.File)
			pv.RenameAsJpeg(pi)
			pi.SaveJpegUpdatedExif(rawExif, img)
		default:
			picinfo.SaveImage(pi.File, img) // todo: need exif support for other formats..
		}
		pv.ThumbGen(pi)
	} else {
		pi.Orient = pi.Orient.Rotate(int(deg))
		pv.SaveExifFile(pi) // does thumbgen
	}
	return nil
}

// SetDateTakenSel sets the DateTaken for selected items, with given day and minute increments between each
func (pv *PixView) SetDateTakenSel(date time.Time, dayInc int, minInc int) {
	pis := pv.CheckSel()
	n := len(pis)
	if n == 0 {
		return
	}
	incSec := time.Second * time.Duration(dayInc*(24*3600)+minInc*60)
	cdt := date
	for _, pi := range pis {
		pv.SetDateTaken(pi, cdt)
		cdt = cdt.Add(incSec)
	}
	pv.FolderFiles = nil
	pv.DirInfo(false) // update -- also saves updated info
}

// SetDateTakenCur sets the DateTaken for single currently-selected file
func (pv *PixView) SetDateTakenCur(date time.Time) error {
	pi := pv.CheckCur()
	if pi == nil {
		return nil
	}
	err := pv.SetDateTaken(pi, date)
	pv.DirInfo(false) // update -- also saves updated info
	return err
}

// SetDateTaken sets the DateTaken for the given image and saves updated Exif metadata.
// Saving the exif requires conversion of non-jpeg format files to Jpeg format.
func (pv *PixView) SetDateTaken(pi *picinfo.Info, date time.Time) error {
	pi.DateTaken = date
	return pv.SaveExifFile(pi)
}

// ImgGridMoveDates moves image dates based on an insert event from ImgGrid
func (pv *PixView) ImgGridMoveDates(idx int) {
	ig := pv.ImgGrid()
	ni := len(ig.Images) - len(pv.Thumbs)
	nf := ig.Images[idx : idx+ni]
	stdate := time.Time{}
	edate := time.Time{}
	if idx > 0 {
		stdate = pv.Info[idx-1].DateTaken
	}
	if idx+ni < len(pv.Info) {
		edate = pv.Info[idx+ni].DateTaken
	}
	var inc time.Duration
	cdt := stdate
	if !stdate.IsZero() && !edate.IsZero() {
		inc = edate.Sub(stdate) / time.Duration(10+ni)
		cdt = stdate.Add(5 * inc)
	} else {
		inc = time.Second * 60
		if stdate.IsZero() {
			cdt = edate.Add(-inc)
			inc = -inc
		} else {
			cdt = cdt.Add(inc)
		}
	}
	for _, f := range nf {
		ii := pv.PicIdxByThumb(f)
		if ii < 0 {
			continue
		}
		pi := pv.Info[ii]
		pv.SetDateTaken(pi, cdt)
		cdt = cdt.Add(inc)
	}
	pv.DirInfo(false)
}

//////////////////////////////////////////////////////////////////
// GoPixViewWindow

// GoPixViewWindow opens an interactive editor of the given Ki tree, at its
// root, returns PixView and window
func GoPixViewWindow(path string) (*PixView, *gi.Window) {
	width := 1280
	height := 920

	win := gi.NewMainWindow("GoPix", "GoPix", width, height)
	vp := win.WinViewport2D()
	updt := vp.UpdateStart()

	mfr := win.SetMainFrame()
	mfr.Lay = gi.LayoutVert

	pv := AddNewPixView(mfr, "pixview")
	pv.Viewport = vp
	pv.Config(path)

	mmen := win.MainMenu
	giv.MainMenuView(pv, win, mmen)

	tb := pv.ToolBar()
	tb.UpdateActions()

	// inClosePrompt := false
	// win.OSWin.SetCloseReqFunc(func(w oswin.Window) {
	// 	// if !pv.Changed {
	// 	// 	win.Close()
	// 	// 	return
	// 	// }
	// 	if inClosePrompt {
	// 		return
	// 	}
	// 	inClosePrompt = true
	// 	gi.ChoiceDialog(vp, gi.DlgOpts{Title: "Close Without Saving?",
	// 		Prompt: "Do you want to save your changes?  If so, Cancel and then Save"},
	// 		[]string{"Close Without Saving", "Cancel"},
	// 		win.This(), func(recv, send ki.Ki, sig int64, data interface{}) {
	// 			switch sig {
	// 			case 0:
	// 				win.Close()
	// 			case 1:
	// 				// default is to do nothing, i.e., cancel
	// 				inClosePrompt = false
	// 			}
	// 		})
	// })

	vp.UpdateEndNoSig(updt)
	win.GoStartEventLoop() // in a separate goroutine
	return pv, win
}

var PixViewProps = ki.Props{
	"EnumType:Flag":    gi.KiT_NodeFlags,
	"background-color": &gi.Prefs.Colors.Background,
	"color":            &gi.Prefs.Colors.Font,
	"max-width":        -1,
	"max-height":       -1,
	"ToolBar": ki.PropSlice{
		{"UpdateFiles", ki.Props{
			"icon":  "update",
			"label": "Update Folders",
		}},
		{"OpenCurDefault", ki.Props{
			"icon":  "file-open",
			"desc":  "open current file (last selected) using OS default app",
			"label": "Open",
		}},
		{"OpenCurGimp", ki.Props{
			"icon":  "file-picture",
			"desc":  "open current file (last selected) using Gimp image editor",
			"label": "Gimp",
		}},
		{"sep-rot", ki.BlankProp{}},
		{"RotateLeftSel", ki.Props{
			"icon":     "rotate-left",
			"label":    "Left",
			"shortcut": "Command+L",
			"desc":     "rotate selected images 90 degrees left",
		}},
		{"RotateRightSel", ki.Props{
			"icon":     "rotate-right",
			"label":    "Right",
			"shortcut": "Command+R",
			"desc":     "rotate selected images 90 degrees right",
		}},
		{"RotateSel", ki.Props{
			"icon":  "rotate-right",
			"label": "Rotate",
			"desc":  "rotate selected images by given number of degrees",
			"Args": ki.PropSlice{
				{"Degrees", ki.Props{}},
			},
		}},
		{"sep-info", ki.BlankProp{}},
		{"InfoCur", ki.Props{
			"icon":  "info",
			"desc":  "show metadata info for current file (last selected)",
			"label": "Info",
		}},
		{"MapCur", ki.Props{
			"icon":  "info",
			"desc":  "show GPS map info for current file (last selected), if available",
			"label": "Map",
		}},
		{"SaveExifSel", ki.Props{
			"icon":  "file-save",
			"desc":  "save any updated exif image metadata for currently selected file(s) if they've been edited -- this will automatically change file to a Jpeg format if it is not already, as that is the only supported exif type (for now)",
			"label": "Save Exif",
		}},
		{"SetDateTakenSel", ki.Props{
			"icon":  "file-save",
			"desc":  "sets the DateTaken, which is how files are sorted, for selected images, with spacing as given by day and minute increments between pictures -- this should only be used for images that don't have an accurate existing date (e.g., scans of old pictures)",
			"label": "Set Date",
			"Args": ki.PropSlice{
				{"Date", ki.Props{}},
				{"Day Increment", ki.Props{}},
				{"Minute Increment", ki.Props{}},
			},
		}},
	},
	"MainMenu": ki.PropSlice{
		{"AppMenu", ki.BlankProp{}},
		{"File", ki.PropSlice{
			{"UpdateFiles", ki.Props{}},
			{"RenameByDate", ki.Props{}},
			{"sep-close", ki.BlankProp{}},
			{"Close Window", ki.BlankProp{}},
		}},
		{"Edit", "Copy Cut Paste Dupe"},
		{"Window", "Windows"},
	},
	"CallMethods": ki.PropSlice{
		{"SetDateTakenCur", ki.Props{
			"icon":  "file-save",
			"desc":  "sets the DateTaken",
			"label": "Set Date",
			"Args": ki.PropSlice{
				{"Date", ki.Props{}},
			},
		}},
	},
}
