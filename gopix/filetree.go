// Copyright (c) 2018, The Gide Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package main

import (
	"image/color"
	"path/filepath"
	"strings"

	"github.com/goki/gi/gi"
	"github.com/goki/gi/gist"
	"github.com/goki/gi/giv"
	"github.com/goki/gi/oswin/dnd"
	"github.com/goki/gi/oswin/mimedata"
	"github.com/goki/gi/units"
	"github.com/goki/gopix/picinfo"
	"github.com/goki/ki/dirs"
	"github.com/goki/ki/ki"
	"github.com/goki/ki/kit"
)

// ParentPixView returns the PixView parent of given node
func ParentPixView(kn ki.Ki) (*PixView, bool) {
	if ki.IsRoot(kn) {
		return nil, false
	}
	var pv *PixView
	kn.FuncUpParent(0, kn, func(k ki.Ki, level int, d any) bool {
		if pvi := k.Embed(KiT_PixView); pvi != nil {
			pv = pvi.(*PixView)
			return false
		}
		return true
	})
	return pv, pv != nil
}

/////////////////////////////////////////////////////////////////////////
// FileTreeView is the GoPix version of the FileTreeView

// FileTreeView is a TreeView that knows how to operate on FileNode nodes
type FileTreeView struct {
	giv.FileTreeView
}

// Drop pops up a menu to determine what specifically to do with dropped items
// satisfies gi.DragNDropper interface and can be overridden by subtypes
func (ftv *FileTreeView) Drop(md mimedata.Mimes, mod dnd.DropMods) {
	ftv.DropCancel() // don't do anything to source
	ftv.PixPaste(md)
}

// PixPaste processes paste / drop operation for pictures
func (ftv *FileTreeView) PixPaste(md mimedata.Mimes) {
	tfn := ftv.FileNode()
	if tfn == nil {
		return
	}
	if !tfn.IsDir() {
		return
	}
	pv, ok := ParentPixView(ftv)
	if !ok {
		return
	}
	var files picinfo.Pics
	nf := len(md)
	for i := 0; i < nf; i++ {
		d := md[i]
		// fmt.Println(string(d.Data))
		fn := filepath.Base(string(d.Data))
		fnext, _ := dirs.SplitExt(fn)
		pi, has := pv.AllInfo[fnext]
		if has {
			files = append(files, pi)
		}
	}

	fnm := strings.ToLower(tfn.Nm)
	switch fnm {
	case "all":
		pv.UntrashFiles(files)
	case "trash":
		pv.TrashFiles(files)
	default:
		pv.LinkToFolder(tfn.Nm, files)
	}
}

var KiT_FileTreeView = kit.Types.AddType(&FileTreeView{}, FileTreeViewProps)

var FileTreeViewProps = ki.Props{
	"EnumType:Flag":    giv.KiT_TreeViewFlags,
	"indent":           units.NewCh(2),
	"spacing":          units.NewCh(.5),
	"border-width":     units.NewPx(0),
	"border-radius":    units.NewPx(0),
	"padding":          units.NewPx(0),
	"margin":           units.NewPx(1),
	"text-align":       gist.AlignLeft,
	"vertical-align":   gist.AlignTop,
	"color":            &gi.Prefs.Colors.Font,
	"background-color": "inherit",
	"no-templates":     true,
	".exec": ki.Props{
		"font-weight": gist.WeightBold,
	},
	".open": ki.Props{
		"font-style": gist.FontItalic,
	},
	".untracked": ki.Props{
		"color": "#808080",
	},
	".modified": ki.Props{
		"color": "#4b7fd1",
	},
	".added": ki.Props{
		"color": "#008800",
	},
	".deleted": ki.Props{
		"color": "#ff4252",
	},
	".conflicted": ki.Props{
		"color": "#ce8020",
	},
	".updated": ki.Props{
		"color": "#008060",
	},
	"#icon": ki.Props{
		"width":   units.NewEm(1),
		"height":  units.NewEm(1),
		"margin":  units.NewPx(0),
		"padding": units.NewPx(0),
		"fill":    &gi.Prefs.Colors.Icon,
		"stroke":  &gi.Prefs.Colors.Font,
	},
	"#branch": ki.Props{
		"icon":             "wedge-down",
		"icon-off":         "wedge-right",
		"margin":           units.NewPx(0),
		"padding":          units.NewPx(0),
		"background-color": color.Transparent,
		"max-width":        units.NewEm(.8),
		"max-height":       units.NewEm(.8),
	},
	"#space": ki.Props{
		"width": units.NewEm(.5),
	},
	"#label": ki.Props{
		"margin":    units.NewPx(0),
		"padding":   units.NewPx(0),
		"min-width": units.NewCh(16),
	},
	"#menu": ki.Props{
		"indicator": "none",
	},
	giv.TreeViewSelectors[giv.TreeViewActive]: ki.Props{},
	giv.TreeViewSelectors[giv.TreeViewSel]: ki.Props{
		"background-color": &gi.Prefs.Colors.Select,
	},
	giv.TreeViewSelectors[giv.TreeViewFocus]: ki.Props{
		"background-color": &gi.Prefs.Colors.Control,
	},
	"CtxtMenuActive": ki.PropSlice{
		{"ShowFileInfo", ki.Props{
			"label": "File Info",
		}},
		{"OpenFileDefault", ki.Props{
			"label": "Open (w/default app)",
		}},
		{"sep-act", ki.BlankProp{}},
		{"DuplicateFiles", ki.Props{
			"label": "Duplicate",
			// "updtfunc": FileTreeInactiveDirFunc,
			"shortcut": gi.KeyFunDuplicate,
		}},
		{"DeleteFiles", ki.Props{
			"label": "Delete",
			"desc":  "Ok to delete file(s)?  This is not undoable and is not moving to trash / recycle bin",
			// "updtfunc": FileTreeInactiveExternFunc,
			"shortcut": gi.KeyFunDelete,
		}},
		{"RenameFiles", ki.Props{
			"label": "Rename",
			"desc":  "Rename file to new file name",
			// "updtfunc": FileTreeInactiveExternFunc,
		}},
		{"sep-new", ki.BlankProp{}},
		{"SortBy", ki.Props{
			"desc": "Choose how to sort files in the directory -- default by Name, optionally can use Modification Time",
			// "updtfunc": FileTreeActiveDirFunc,
			"Args": ki.PropSlice{
				{"Modification Time", ki.Props{}},
			},
		}},
		{"NewFolder", ki.Props{
			"label":    "New Folder...",
			"desc":     "make a new folder within this folder",
			"shortcut": gi.KeyFunInsertAfter,
			// "updtfunc": FileTreeActiveDirFunc,
			"Args": ki.PropSlice{
				{"Folder Name", ki.Props{
					"width": 60,
				}},
			},
		}},
	},
}
