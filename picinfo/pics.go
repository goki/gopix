// Copyright (c) 2020, The Goki Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package picinfo

import (
	"bufio"
	"encoding/json"
	"log"
	"os"
	"sort"

	"github.com/goki/ki/dirs"
)

// Pics is a slice of Info for a list of pictures
type Pics []*Info

// SortByDate sorts the pictures by date taken
func (pc Pics) SortByDate(ascending bool) {
	if ascending {
		sort.Slice(pc, func(i, j int) bool {
			return pc[i].DateTaken.Before(pc[j].DateTaken)
		})
	} else {
		sort.Slice(pc, func(i, j int) bool {
			return pc[j].DateTaken.Before(pc[i].DateTaken)
		})
	}
}

// Thumbs returns the list of thumbs for this set of pictures
func (pc Pics) Thumbs() []string {
	th := make([]string, len(pc))
	for i, pi := range pc {
		th[i] = pi.Thumb
	}
	return th
}

// IdxByFile returns the index of given File name
func (pc Pics) IdxByFile(fname string) int {
	for i, pi := range pc {
		if pi.File == fname {
			return i
		}
	}
	return -1
}

// IdxByThumb returns the index of given Thumb name
func (pc Pics) IdxByThumb(tname string) int {
	for i, pi := range pc {
		if pi.Thumb == tname {
			return i
		}
	}
	return -1
}

//////////////////////////////////////////////////////
// PicMap

// PicMap is a map of Info for a collection of pictures, with the key being
// the file name *without extension*, so it can be used with thumb or picture
// filenames, and is consistent if the file extension is changed.
type PicMap map[string]*Info

// InfoByName returns the Info record based on file name (strips extension)
func (pm *PicMap) InfoByName(fname string) (*Info, bool) {
	fnext, _ := dirs.SplitExt(fname)
	pi, has := (*pm)[fnext]
	return pi, has
}

// Set sets the Info into the map
func (pm *PicMap) Set(pi *Info) {
	fnext, _ := dirs.SplitExt(pi.File)
	(*pm)[fnext] = pi
}

// OpenJSON opens from a JSON encoded file.
// Logs any errors.
func (pm *PicMap) OpenJSON(fname string) error {
	*pm = make(map[string]*Info)

	f, err := os.Open(fname)
	defer f.Close()
	if err != nil {
		log.Println(err)
		return err
	}

	d := json.NewDecoder(f)
	err = d.Decode(pm)
	if err != nil {
		log.Println(err)
	}
	return err
}

// SaveJSON saves to a JSON encoded file
func (pm *PicMap) SaveJSON(fname string) error {
	f, err := os.Create(fname)
	defer f.Close()
	if err != nil {
		log.Println(err)
		return err
	}

	fb := bufio.NewWriter(f) // this makes a HUGE difference in write performance!
	defer fb.Flush()

	e := json.NewEncoder(fb)
	e.SetIndent("", "\t")
	err = e.Encode(*pm)
	if err != nil {
		log.Println(err)
	}
	return err
}

// SetFileThumb sets the File and Thumb names based on given directories.
// These are not saved in the JSON files to conserve space.
// Thumb is always set to a .jpg version of base file name.
func (pm *PicMap) SetFileThumb(adir, tdir string) {
	for fnext, pi := range *pm {
		if pi.Ext == "" { // old record!
			fnb, ext := dirs.SplitExt(fnext)
			pi.Ext = ext
			delete(*pm, fnext)
			(*pm)[fnb] = pi
			fnext = fnb
		}
		pi.SetFileThumbFmBase(fnext, adir, tdir)
	}
}
