// Copyright (c) 2018, The Gide Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package main

import (
	"github.com/goki/gi/giv"
	"github.com/goki/ki/ki"
	"github.com/goki/ki/kit"
)

/////////////////////////////////////////////////////////////////////////
// FileTreeView is the GoPix version of the FileTreeView

// FileTreeView is a TreeView that knows how to operate on FileNode nodes
type FileTreeView struct {
	giv.FileTreeView
}

var FileTreeViewProps map[string]interface{}
var FileNodeProps map[string]interface{}

var KiT_FileTreeView = kit.Types.AddType(&FileTreeView{}, nil)

func init() {
	FileTreeViewProps = make(ki.Props, len(giv.FileTreeViewProps))
	ki.CopyProps(&FileTreeViewProps, giv.FileTreeViewProps, true)
	// cm := FileTreeViewProps["CtxtMenuActive"].(ki.PropSlice)
	// cm = append(ki.PropSlice{
	// 	{"ExecCmdFiles", ki.Props{
	// 		"label":        "Exec Cmd",
	// 		"submenu-func": giv.SubMenuFunc(FileTreeViewExecCmds),
	// 		"Args": ki.PropSlice{
	// 			{"Cmd Name", ki.Props{}},
	// 		},
	// 	}},
	// 	{"EditFiles", ki.Props{
	// 		"label":    "Edit",
	// 		"updtfunc": FileTreeInactiveDirFunc,
	// 	}},
	// 	{"SetRunExec", ki.Props{
	// 		"label":    "Set Run Exec",
	// 		"updtfunc": FileTreeActiveExecFunc,
	// 	}},
	// 	{"sep-view", ki.BlankProp{}},
	// }, cm...)
	// FileTreeViewProps["CtxtMenuActive"] = cm
	kit.Types.SetProps(KiT_FileTreeView, FileTreeViewProps)
}
