// Copyright (c) 2020, The GoKi Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package main

import (
	"sync"

	"github.com/goki/gi/gi"
	"github.com/goki/gi/giv"
	"github.com/goki/gi/oswin"
	"github.com/goki/gopix/imgrid"
	"github.com/goki/ki/ki"
	"github.com/goki/ki/kit"
	"github.com/goki/pi/filecat"
)

// PixView shows a picture viewer
type PixView struct {
	gi.Frame
	ImageDir gi.FileName    `desc:"directory with the images"`
	Folder   string         `desc:"current folder"`
	Files    giv.FileTree   `desc:"all the files in the project directory and subdirectories"`
	Images   []string       `view:"-" desc:"desc list of all image files in current folder"`
	Thumbs   []string       `view:"-" desc:"desc list of all thumb files"`
	WaitGp   sync.WaitGroup `view:"-" desc:"wait group for synchronizing threaded layer calls"`
}

var KiT_PixView = kit.Types.AddType(&PixView{}, PixViewProps)

// AddNewPixView adds a new pixview to given parent node, with given name.
func AddNewPixView(parent ki.Ki, name string) *PixView {
	return parent.AddNewChild(KiT_PixView, name).(*PixView)
}

// UpdateFiles updates the list of files saved in project
func (pv *PixView) UpdateFiles() {
	pv.Files.OpenPath(string(pv.ImageDir))
	ft := pv.FileTreeView()
	ft.SetFullReRender()
	ft.UpdateSig()
}

// Config configures the widget
func (pv *PixView) Config() {
	if pv.HasChildren() {
		return
	}
	updt := pv.UpdateStart()
	defer pv.UpdateEnd(updt)

	pv.Folder = "all"
	pv.Lay = gi.LayoutVert
	pv.SetProp("spacing", gi.StdDialogVSpaceUnits)
	gi.AddNewToolBar(pv, "toolbar")
	split := gi.AddNewSplitView(pv, "splitview")

	ftfr := gi.AddNewFrame(split, "filetree", gi.LayoutVert)
	ft := ftfr.AddNewChild(KiT_FileTreeView, "filetree").(*FileTreeView)
	ft.OpenDepth = 4

	ig := imgrid.AddNewImgGrid(split, "imgrid")
	ig.Config()

	split.SetSplits(.2, .8)

	pv.UpdateFiles()
	ft.SetRootNode(&pv.Files)
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
}

// FileNodeSelected is called whenever tree browser has file node selected
func (pv *PixView) FileNodeSelected(fn *giv.FileNode, tvn *FileTreeView) {
	// if fn.IsDir() {
	// } else {
	// }
}

// FileNodeOpened is called whenever file node is double-clicked in file tree
func (pv *PixView) FileNodeOpened(fn *giv.FileNode, tvn *FileTreeView) {
	switch fn.Info.Cat {
	case filecat.Folder:
		if !fn.IsOpen() {
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

// SplitView returns the main SplitView
func (pv *PixView) SplitView() *gi.SplitView {
	return pv.ChildByName("splitview", 1).(*gi.SplitView)
}

// FileTreeView returns the main FileTreeView
func (pv *PixView) FileTreeView() *FileTreeView {
	return pv.SplitView().Child(0).Child(0).(*FileTreeView)
}

// ImgGrid returns the ImgGrid
func (pv *PixView) ImgGrid() *imgrid.ImgGrid {
	return pv.SplitView().Child(1).(*imgrid.ImgGrid)
}

// ToolBar returns the toolbar widget
func (pv *PixView) ToolBar() *gi.ToolBar {
	return pv.ChildByName("toolbar", 0).(*gi.ToolBar)
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

var PixViewProps = ki.Props{
	"EnumType:Flag":    gi.KiT_NodeFlags,
	"background-color": &gi.Prefs.Colors.Background,
	"color":            &gi.Prefs.Colors.Font,
	"max-width":        -1,
	"max-height":       -1,
	"ToolBar": ki.PropSlice{
		{"UpdateFiles", ki.Props{
			"icon": "update",
		}},
	},
	"MainMenu": ki.PropSlice{
		{"AppMenu", ki.BlankProp{}},
		{"File", ki.PropSlice{
			{"UpdateFiles", ki.Props{}},
			{"sep-close", ki.BlankProp{}},
			{"Close Window", ki.BlankProp{}},
		}},
		{"Edit", "Copy Cut Paste Dupe"},
		{"Window", "Windows"},
	},
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
	pv.ImageDir = gi.FileName(path)
	pv.Viewport = vp
	pv.Config()

	mmen := win.MainMenu
	giv.MainMenuView(pv, win, mmen)

	tb := pv.ToolBar()
	tb.UpdateActions()

	inClosePrompt := false
	win.OSWin.SetCloseReqFunc(func(w oswin.Window) {
		// if !pv.Changed {
		// 	win.Close()
		// 	return
		// }
		if inClosePrompt {
			return
		}
		inClosePrompt = true
		gi.ChoiceDialog(vp, gi.DlgOpts{Title: "Close Without Saving?",
			Prompt: "Do you want to save your changes?  If so, Cancel and then Save"},
			[]string{"Close Without Saving", "Cancel"},
			win.This(), func(recv, send ki.Ki, sig int64, data interface{}) {
				switch sig {
				case 0:
					win.Close()
				case 1:
					// default is to do nothing, i.e., cancel
					inClosePrompt = false
				}
			})
	})

	vp.UpdateEndNoSig(updt)
	win.GoStartEventLoop() // in a separate goroutine
	return pv, win
}
