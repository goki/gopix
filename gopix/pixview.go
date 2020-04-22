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

	"github.com/goki/gi/gi"
	"github.com/goki/gi/giv"
	"github.com/goki/gi/oswin"
	"github.com/goki/gopix/imgrid"
	"github.com/goki/gopix/picinfo"
	"github.com/goki/ki/dirs"
	"github.com/goki/ki/ki"
	"github.com/goki/ki/kit"
	"github.com/goki/pi/filecat"
)

// PixView shows a picture viewer
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
	ig.Config()
	ig.InfoFunc = pv.FileInfo

	pic := tv.AddNewTab(gi.KiT_Bitmap, "Current").(*gi.Bitmap)
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
		// igg, _ := send.Embed(imgrid.KiT_ImgGrid).(*imgrid.ImgGrid)
		pvv, _ := recv.Embed(KiT_PixView).(*PixView)
		idx := data.(int)
		if idx < 0 || idx >= len(pv.Info) {
			return
		}
		fn := filepath.Base(pv.Info[idx].File)
		switch imgrid.ImgGridSignals(sig) {
		case imgrid.ImgGridDeleted:
			if pvv.Folder == "All" {
				pvv.TrashFiles([]string{fn})
			} else {
				pvv.DeleteInFolder(pvv.Folder, []string{fn}) // this works for Trash too -- permanent..
			}
			pvv.PicDeleteAt(idx)
		case imgrid.ImgGridInserted:
			// we don't really have anything useful to do here..
		// 	if pvv.Folder == "Trash" {
		// 		pvv.UntrashFiles([]string{fn})
		// 	} else {
		// 		// todo: duplicate
		// 	}
		// 	pvv.PicInsertAt(idx, []string{""}) // todo: not really used or sensible
		case imgrid.ImgGridDoubleClicked:
			pvv.ViewFile(fn)
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

// CurBitmap returns the Bitmap for viewing the current file
func (pv *PixView) CurBitmap() *gi.Bitmap {
	return pv.Tabs().TabByName("Current").(*gi.Bitmap)
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
		pv.DirInfo()
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

// todo: rename all files to date-stamp + uniq ext

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
func (pv *PixView) ViewFile(fname string) {
	pv.Tabs().SelectTabByName("Current")
	bm := pv.CurBitmap()
	pi, has := pv.AllInfo[fname]
	if !has {
		log.Printf("weird, no info for: %v\n", fname)
		return
	}
	pv.CurFile = fname
	img, err := picinfo.OpenImage(pi.File)
	if err != nil {
		log.Println(err)
		return
	}
	img = picinfo.OrientImage(img, pi.Orient)
	bm.SetImage(img, 0, 0)
}

// CheckCur checks that current file name is set and exists in AllInfo
// returns false if not, and opens a prompt dialog
func (pv *PixView) CheckCur() bool {
	bad := false
	if pv.CurFile == "" {
		bad = true
	}
	if !bad {
		_, has := pv.AllInfo[pv.CurFile]
		bad = !has
	}
	if bad {
		gi.PromptDialog(nil, gi.DlgOpts{Title: "No Current File", Prompt: "Please select a valid image file and retry"}, gi.AddOk, gi.NoCancel, nil, nil)
		return false
	}
	return true
}

// OpenCurDefault opens the current file using the OS default open command
func (pv *PixView) OpenCurDefault() {
	if !pv.CheckCur() {
		return
	}
	pv.OpenDefault(pv.CurFile)
}

// OpenDefault opens the given file using the OS default open command
func (pv *PixView) OpenDefault(fname string) {
	adir := filepath.Join(pv.ImageDir, "All")
	afn := filepath.Join(adir, fname)
	cstr := giv.OSOpenCommand()
	cmd := exec.Command(cstr, afn)
	out, _ := cmd.CombinedOutput()
	fmt.Printf("%s\n", out)
}

// OpenCurGimp opens the current file using gimp
func (pv *PixView) OpenCurGimp() {
	if !pv.CheckCur() {
		return
	}
	pv.OpenGimp(pv.CurFile)
}

// OpenGimp opens the given file using gimp
func (pv *PixView) OpenGimp(fname string) {
	adir := filepath.Join(pv.ImageDir, "All")
	afn := filepath.Join(adir, fname)
	var cmd *exec.Cmd
	if oswin.TheApp.Platform() == oswin.MacOS {
		cmd = exec.Command("open", "-a", "Gimp", afn)
	} else {
		cmd = exec.Command("gimp", afn)
	}
	out, _ := cmd.CombinedOutput()
	fmt.Printf("%s\n", out)
}

// InfoCur shows metadata info about current file
func (pv *PixView) InfoCur() {
	if !pv.CheckCur() {
		return
	}
	pv.InfoFile(pv.CurFile)
}

// InfoFile shows metadata info about current file
func (pv *PixView) InfoFile(fname string) {
	pi, has := pv.AllInfo[fname]
	if !has {
		log.Printf("weird, no info for: %v\n", fname)
		return
	}
	giv.StructViewDialog(pv.Viewport, pi, giv.DlgOpts{Title: "Picture Info: " + pi.File}, nil, nil)
}

// FileInfo shows info for given file index -- callback for ImgGrid
func (pv *PixView) FileInfo(idx int) {
	pi := pv.Info[idx]
	giv.StructViewDialog(pv.Viewport, pi, giv.DlgOpts{Title: "Picture Info: " + pi.File}, nil, nil)
}

// MapCur shows GPS coordinates for current file on google maps
func (pv *PixView) MapCur() {
	if !pv.CheckCur() {
		return
	}
	pv.MapFile(pv.CurFile)
}

// MapFile shows GPS coordinates for file on google maps
func (pv *PixView) MapFile(fname string) {
	pi, has := pv.AllInfo[fname]
	if !has {
		log.Printf("weird, no info for: %v\n", fname)
		return
	}
	if pi.GPSLoc.Lat == 0 && pi.GPSLoc.Long == 0 {
		gi.PromptDialog(nil, gi.DlgOpts{Title: "No GPS Location", Prompt: "That file does not have a GPS location"}, gi.AddOk, gi.NoCancel, nil, nil)
		return
	}
	url := fmt.Sprintf("https://www.google.com/maps/search/?api=1&query=%g,%g", pi.GPSLoc.Lat, pi.GPSLoc.Long)
	oswin.TheApp.OpenURL(url)
}

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
}
