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
	"sync"
	"time"

	"github.com/anthonynsimon/bild/transform"
	"github.com/goki/gi/gi"
	"github.com/goki/gi/giv"
	"github.com/goki/gi/oswin"
	"github.com/goki/gi/oswin/mouse"
	"github.com/goki/gopix/imgrid"
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

	// current file base name (no path, no ext) -- viewed in Current bitmap or last selected in ImgGrid
	CurFile string `desc:"current file base name (no path, no ext) -- viewed in Current bitmap or last selected in ImgGrid"`

	// index of current file in Info list
	CurIdx int `desc:"index of current file in Info list"`

	// directory with the images
	ImageDir string `desc:"directory with the images"`

	// current folder
	Folder string `desc:"current folder"`

	// list of all folders, excluding All and Trash
	Folders []string `desc:"list of all folders, excluding All and Trash"`

	// list of all files in all Folders -- used for e.g., large renames
	FolderFiles []map[string]struct{} `desc:"list of all files in all Folders -- used for e.g., large renames"`

	// all the files in the project directory and subdirectories
	Files giv.FileTree `desc:"all the files in the project directory and subdirectories"`

	// info for all the pictures in current folder
	Info picinfo.Pics `desc:"info for all the pictures in current folder"`

	// map of info for all files
	AllInfo picinfo.PicMap `desc:"map of info for all files"`

	// mutex protecting AllInfo local access within processing steps
	AllMu sync.Mutex `desc:"mutex protecting AllInfo local access within processing steps"`

	// mutex for any big task involving updating AllInfo
	UpdtMu sync.Mutex `desc:"mutex for any big task involving updating AllInfo"`

	// desc list of all thumb files in current folder -- sent to ImgGrid -- must be in 1-to-1 order with Info
	Thumbs []string `view:"-" desc:"desc list of all thumb files in current folder -- sent to ImgGrid -- must be in 1-to-1 order with Info"`

	// wait group for synchronizing threaded layer calls
	WaitGp sync.WaitGroup `view:"-" desc:"wait group for synchronizing threaded layer calls"`

	// parallel progress monitor
	PProg *gi.ProgressBar `view:"-" desc:"parallel progress monitor"`
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

	pv.Lay = gi.LayoutVert
	pv.SetProp("spacing", gi.StdDialogVSpaceUnits)
	tbar := gi.AddNewLayout(pv, "topbar", gi.LayoutHoriz)
	tbar.SetStretchMaxWidth()
	gi.AddNewToolBar(tbar, "toolbar")
	pv.PProg = gi.AddNewProgressBar(tbar, "progress")
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

	pic := tv.AddNewTab(KiT_ImgView, "Current").(*ImgView)
	pic.PixView = pv
	pic.SetStretchMax()

	split.SetSplits(.1, .9)

	pv.UpdateFiles()
	ft.SetRootNode(&pv.Files)

	pv.ConfigToolbar()

	ft.TreeViewSig.Connect(pv.This(), func(recv, send ki.Ki, sig int64, data any) {
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
	ig.WidgetSig.Connect(pv.This(), func(recv, send ki.Ki, sig int64, data any) {
		if sig == int64(gi.WidgetSelected) {
			pvv, _ := recv.Embed(KiT_PixView).(*PixView)
			idx := data.(int)
			if idx >= 0 && idx < len(pv.Info) {
				pvv.SetCurFile(pv.Info[idx], idx)
			}
		}
	})
	ig.ImageSig.Connect(pv.This(), func(recv, send ki.Ki, sig int64, data any) {
		igg, _ := send.Embed(imgrid.KiT_ImgGrid).(*imgrid.ImgGrid)
		pvv, _ := recv.Embed(KiT_PixView).(*PixView)
		idx := data.(int)
		if idx < 0 || idx >= len(pv.Info) {
			return
		}
		pi := pv.Info[idx]
		pics := picinfo.Pics{pi}
		switch imgrid.ImgGridSignals(sig) {
		case imgrid.ImgGridDeleted:
			if pvv.Folder == "All" {
				pvv.TrashFiles(pics)
			} else {
				pvv.DeleteInFolder(pvv.Folder, pics) // this works for Trash too -- permanent..
			}
			pvv.PicDeleteAt(idx)
		case imgrid.ImgGridInserted:
			if pvv.Folder == "Trash" {
				igg.Images = sliceclone.String(pv.Thumbs)
				return
			}
			pvv.ImgGridMoveDates(idx)
		case imgrid.ImgGridDoubleClicked:
			pvv.ViewFile(pi, idx)
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
func (pv *PixView) CurImgView() *ImgView {
	return pv.Tabs().TabByName("Current").(*ImgView)
}

// ToolBar returns the toolbar widget
func (pv *PixView) ToolBar() *gi.ToolBar {
	return pv.ChildByName("topbar", 0).ChildByName("toolbar", 0).(*gi.ToolBar)
}

// ProgBar returns the progress indicator
func (pv *PixView) ProgBar() *gi.ScrollBar {
	return pv.ChildByName("topbar", 0).ChildByName("progress", 1).(*gi.ScrollBar)
}

// SetCurFile sets CurFile based on given Info record, at given index in pi.Info
func (pv *PixView) SetCurFile(pi *picinfo.Info, idx int) {
	pv.CurFile = pi.FileBase()
	pv.CurIdx = idx
}

// PicDeleteAt deletes active Info / Thumb image at given index
func (pv *PixView) PicDeleteAt(idx int) {
	pv.Info = append(pv.Info[:idx], pv.Info[idx+1:]...)
	pv.Thumbs = append(pv.Thumbs[:idx], pv.Thumbs[idx+1:]...)
}

// FileNodeSelected is called whenever tree browser has file node selected
func (pv *PixView) FileNodeSelected(fn *giv.FileNode, tvn *FileTreeView) {
	if fn.IsDir() {
		pv.Folder = fn.Nm
		pv.Tabs().SelectTabByName("Images")
		pv.UpdtMu.Lock()
		pv.DirInfo(true) // reset
		pv.UpdtMu.Unlock()
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
	if idx >= nf || idx < 0 {
		return
	}
	pi := pv.Info[idx]
	m.AddAction(gi.ActOpts{Label: "Info", Data: idx},
		pv.This(), func(recv, send ki.Ki, sig int64, data any) {
			pv.InfoFile(pi)
		})
	m.AddAction(gi.ActOpts{Label: "SetDate", Data: idx},
		pv.This(), func(recv, send ki.Ki, sig int64, data any) {
			pv.SetCurFile(pi, idx)
			giv.CallMethod(pv, "SetDateTakenCur", pv.Viewport)
		})
	m.AddSeparator("clip")
	m.AddAction(gi.ActOpts{Label: "Copy", Data: idx},
		pv.This(), func(recv, send ki.Ki, sig int64, data any) {
			ig.CopyIdxs(true)
		})
	m.AddAction(gi.ActOpts{Label: "Cut", Data: idx},
		pv.This(), func(recv, send ki.Ki, sig int64, data any) {
			ig.CutIdxs()
		})
	m.AddAction(gi.ActOpts{Label: "Paste", Data: idx},
		pv.This(), func(recv, send ki.Ki, sig int64, data any) {
			ig.PasteIdx(data.(int))
		})
	m.AddAction(gi.ActOpts{Label: "Duplicate", Data: idx},
		pv.This(), func(recv, send ki.Ki, sig int64, data any) {
			pv.Duplicate(pi)
		})
	m.AddAction(gi.ActOpts{Label: "Delete", Data: idx},
		pv.This(), func(recv, send ki.Ki, sig int64, data any) {
			ig.CutIdxs()
		})
}

//////////////////////////////////////////////////////////////////////////////////
//  file functions

// LinkToFolder creates links in given folder o given files in ../All
func (pv *PixView) LinkToFolder(fnm string, files picinfo.Pics) {
	tdir := filepath.Join(pv.ImageDir, fnm)
	for _, pi := range files {
		fn := filepath.Base(pi.File)
		lf := filepath.Join(tdir, fn)
		sf := filepath.Join("../All", fn)
		err := os.Symlink(sf, lf)
		if err != nil {
			log.Println(err)
		}
	}
}

// RenameFile renames file in All and any folders where it might be linked
// input is just the file name, no path (base is taken anyway in precaution)
func (pv *PixView) RenameFile(oldnm, newnm string) {
	oldnm = filepath.Base(oldnm)
	newnm = filepath.Base(newnm)
	adir := filepath.Join(pv.ImageDir, "All")
	aofn := filepath.Join(adir, oldnm)
	anfn := filepath.Join(adir, newnm)
	os.Rename(aofn, anfn)

	sf := filepath.Join("../All", newnm)
	for i, fld := range pv.Folders {
		fdir := filepath.Join(pv.ImageDir, fld)
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
func (pv *PixView) DeleteInFolder(fld string, files picinfo.Pics) {
	tdir := filepath.Join(pv.ImageDir, fld)
	for _, pi := range files {
		fn := filepath.Base(pi.File)
		lf := filepath.Join(tdir, fn)
		err := os.Remove(lf)
		if err != nil {
			log.Println(err)
		}
		if fld == "Trash" {
			os.Remove(pi.Thumb)
			fnb := pi.FileBase()
			delete(pv.AllInfo, fnb)
		}
	}
}

// TrashFiles moves given files from All to Trash, and removes symlinks from
// any folders.  Does not delete from AllFiles or delete Thumb.
// These should be full base filenames (with extensions, but no path).
func (pv *PixView) TrashFiles(files picinfo.Pics) {
	adir := filepath.Join(pv.ImageDir, "All")
	tdir := filepath.Join(pv.ImageDir, "Trash")
	os.MkdirAll(tdir, 0775)
	for _, pi := range files {
		fn := filepath.Base(pi.File)
		tfn := filepath.Join(tdir, fn)
		afn := filepath.Join(adir, fn)
		err := os.Rename(afn, tfn)
		if err != nil {
			log.Println(err)
		}
		pv.DeleteFromFolders(fn)
	}
}

// DeleteFromFolders deletes given file name (with extension, no path)
// from all Folders.  Just does remove and ignores the errors.
func (pv *PixView) DeleteFromFolders(fname string) {
	fn := filepath.Base(fname)       // make sure
	for _, fld := range pv.Folders { // easier to just try it..
		fdir := filepath.Join(pv.ImageDir, fld)
		os.Remove(filepath.Join(fdir, fn))
	}
}

// UntrashFiles moves given files from Trash to All (recover from trash)
func (pv *PixView) UntrashFiles(files picinfo.Pics) {
	adir := filepath.Join(pv.ImageDir, "All")
	tdir := filepath.Join(pv.ImageDir, "Trash")
	os.MkdirAll(tdir, 0775)
	for _, pi := range files {
		fn := filepath.Base(pi.File)
		tfn := filepath.Join(tdir, fn)
		afn := filepath.Join(adir, fn)
		err := os.Rename(tfn, afn)
		if err != nil {
			log.Println(err)
		}
	}
}

// DeleteCurPic deletes the currently-viewed file (CurFile, CurIdx)
func (pv *PixView) DeleteCurPic() {
	pic := pv.CheckCur()
	if pic == nil {
		return
	}
	pics := picinfo.Pics{pic}
	if pv.Folder == "All" {
		pv.TrashFiles(pics)
	} else {
		pv.DeleteInFolder(pv.Folder, pics) // this works for Trash too -- permanent..
	}
	pv.PicDeleteAt(pv.CurIdx)
}

// ViewFile views given file in the full-size bitmap view
func (pv *PixView) ViewFile(pi *picinfo.Info, idx int) {
	ig := pv.ImgGrid()
	ig.SelectIdxAction(idx, mouse.SelectOne) // ensure grid always shows current
	pv.Tabs().SelectTabByName("Current")
	pv.SetCurFile(pi, idx)
	iv := pv.CurImgView()
	iv.SetInfo(pi)
}

// ViewNext views the next file in the list relative to CurIdx.
// returns false if at end
func (pv *PixView) ViewNext() bool {
	nf := len(pv.Info)
	if nf == 0 {
		return false
	}
	nx := pv.CurIdx + 1
	if nx >= nf {
		return false
	}
	ig := pv.ImgGrid()
	ig.SelectIdxAction(nx, mouse.SelectOne) // ensure grid always shows current
	pi := pv.Info[nx]
	pv.Tabs().SelectTabByName("Current")
	pv.SetCurFile(pi, nx)
	iv := pv.CurImgView()
	iv.SetInfo(pi)
	return true
}

// ViewPrev views the previous file in the list relative to CurIdx.
// returns false if at start
func (pv *PixView) ViewPrev() bool {
	nf := len(pv.Info)
	if nf == 0 {
		return true
	}
	if pv.CurIdx >= nf {
		pv.CurIdx = nf - 1
	}
	nx := pv.CurIdx - 1
	if nx < 0 {
		return true
	}
	ig := pv.ImgGrid()
	ig.SelectIdxAction(nx, mouse.SelectOne) // ensure grid always shows current
	pi := pv.Info[nx]
	pv.Tabs().SelectTabByName("Current")
	pv.SetCurFile(pi, nx)
	iv := pv.CurImgView()
	iv.SetInfo(pi)
	return true
}

// ViewRefresh re-displays current image (i.e., after change)
func (pv *PixView) ViewRefresh() {
	nf := len(pv.Info)
	if nf == 0 {
		return
	}
	if pv.CurIdx >= nf {
		pv.CurIdx = nf - 1
	}
	if pv.CurIdx < 0 {
		pv.CurIdx = 0
	}
	pi := pv.Info[pv.CurIdx]
	pv.Tabs().SelectTabByName("Current")
	pv.SetCurFile(pi, pv.CurIdx)
	iv := pv.CurImgView()
	iv.SetInfo(pi)
}

// Duplicate duplicates image
func (pv *PixView) Duplicate(pi *picinfo.Info) error {
	pv.UpdtMu.Lock()
	defer pv.UpdtMu.Unlock()

	nfn, n := pv.UniqueNameNumber(pi.DateTaken, pi.Number)
	npi := &picinfo.Info{}
	*npi = *pi
	adir := filepath.Join(pv.ImageDir, "All")
	tdir := pv.ThumbDir()
	npi.Number = n
	npi.SetFileThumbFmBase(nfn, adir, tdir)
	giv.CopyFile(npi.File, pi.File, 0664)
	npi.UpdateFileMod()
	giv.CopyFile(npi.Thumb, pi.Thumb, 0664)
	pv.AllInfo[nfn] = npi
	if pv.Folder != "All" {
		pv.LinkToFolder(pv.Folder, picinfo.Pics{npi})
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

// NewFolder creates a new folder of the given name
func (pv *PixView) NewFolder(fname string) {
	np := filepath.Join(pv.ImageDir, fname)
	os.Mkdir(np, 0775)
	pv.UpdateFolders()
}

// EmptyTrash deletes all files in the trash
func (pv *PixView) EmptyTrash() {
	pv.UpdtMu.Lock()
	defer pv.UpdtMu.Unlock()

	pv.Folder = "Trash"
	pv.DirInfo(true)
	pv.DeleteInFolder(pv.Folder, pv.Info)
	pv.DirInfo(true)
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
	pv.UpdtMu.Lock()
	defer pv.UpdtMu.Unlock()

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
	pv.UpdtMu.Lock()
	defer pv.UpdtMu.Unlock()

	pis := pv.CheckSel()
	n := len(pis)
	if n == 0 {
		return
	}
	pv.PProg.Start(len(pis))
	for _, pi := range pis {
		pv.RotateImage(pi, deg)
		pv.PProg.ProgStep()
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
	pv.UpdtMu.Lock()
	defer pv.UpdtMu.Unlock()

	pis := pv.CheckSel()
	n := len(pis)
	if n == 0 {
		return
	}
	incSec := time.Second * time.Duration(dayInc*(24*3600)+minInc*60)
	cdt := date
	pv.PProg.Start(len(pis))
	for _, pi := range pis {
		pv.SetDateTaken(pi, cdt)
		cdt = cdt.Add(incSec)
		pv.PProg.ProgStep()
	}
	pv.FolderFiles = nil
	pv.DirInfo(false) // update -- also saves updated info
}

// SetDateTakenCur sets the DateTaken for single currently-selected file
func (pv *PixView) SetDateTakenCur(date time.Time) error {
	pv.UpdtMu.Lock()
	defer pv.UpdtMu.Unlock()

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
	pv.UpdtMu.Lock()
	defer pv.UpdtMu.Unlock()

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
		ii := pv.Info.IdxByThumb(f)
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
	pv.UpdtMu.Lock()
	win.GoStartEventLoop() // in a separate goroutine
	pv.UniquifyBaseNames()
	pv.OpenAllInfo()
	pv.UpdtMu.Unlock()
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
		{"sep-fold", ki.BlankProp{}},
		{"NewFolder", ki.Props{
			"icon": "folder-plus",
			"desc": "Create new folder with given name",
			"Args": ki.PropSlice{
				{"Folder Name", ki.Props{}},
			},
		}},
		{"EmptyTrash", ki.Props{
			"icon":    "trash",
			"desc":    "Empty the Trash folder -- <b>Permanently</b> deletes the trashed pics!",
			"confirm": true,
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
			{"RenameByDate", ki.Props{
				"desc":    "Rename files by their date taken -- be sure to click on All first to ensure current files are loaded.",
				"confirm": true,
			}},
			{"CleanAllInfo", ki.Props{
				"desc": "Clean the info.json list of all files -- be sure to click on All dir first to make sure everything is loaded first.  Dry Run does not do anything -- just reports what would be done.",
				"Args": ki.PropSlice{
					{"Dry Run", ki.Props{}},
				},
			}},
			{"CleanDupes", ki.Props{
				"desc": "Check for duplicates and move one of them to the trash -- go to the Trash folder afterward and select all and delete to permanently delete.",
				"Args": ki.PropSlice{
					{"Dry Run", ki.Props{}},
				},
			}},
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
