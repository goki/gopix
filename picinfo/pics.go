// Copyright (c) 2020, The GoKi Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package picinfo

import (
	"bufio"
	"encoding/json"
	"log"
	"os"
	"path/filepath"
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

//////////////////////////////////////////////////////
// PicMap

// PicMap is a map of Info for a collection of pictures
type PicMap map[string]*Info

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
	for fn, pi := range *pm {
		fnext, _ := dirs.SplitExt(fn)
		ffn := filepath.Join(adir, fn)
		tfn := filepath.Join(tdir, fnext+".jpg")
		pi.File = ffn
		pi.Thumb = tfn
	}
}
