// Copyright (c) 2020, The GoKi Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package picinfo

import (
	"fmt"
	"image"
	"path/filepath"
	"time"

	"github.com/goki/ki/dirs"
	"github.com/goki/ki/kit"
	"github.com/goki/pi/filecat"
)

// Info is the information about a picture / video file
type Info struct {

	// full path to image file name
	File string `json:"-" desc:"full path to image file name"`

	// extension of the file name
	Ext string `desc:"extension of the file name"`

	// image description -- can contain arbitrary user comments -- ascii encoded
	Desc string `desc:"image description -- can contain arbitrary user comments -- ascii encoded"`

	// date when image file was modified
	FileMod time.Time `desc:"date when image file was modified"`

	// supported type of image file, decoded from extension, using gopi/filecat system
	Sup filecat.Supported `desc:"supported type of image file, decoded from extension, using gopi/filecat system"`

	// if there are multiple files taken at the same time, e.g., in a Burst, this is the number
	Number int `desc:"if there are multiple files taken at the same time, e.g., in a Burst, this is the number"`

	// size of image in raw pixels
	Size image.Point `desc:"size of image in raw pixels"`

	// number of bits in each color component (e.g., 8 is typical)
	Depth int `desc:"number of bits in each color component (e.g., 8 is typical)"`

	// orientation of the image using exif standards that include rotation and mirroring
	Orient Orientations `desc:"orientation of the image using exif standards that include rotation and mirroring"`

	// date when the image / video was taken
	DateTaken time.Time `desc:"date when the image / video was taken"`

	// date when image was last modified / edited
	DateMod time.Time `desc:"date when image was last modified / edited"`

	// GPS coordinates of location of shot
	GPSLoc GPSCoord `view:"inline" desc:"GPS coordinates of location of shot"`

	// GPS misc additional data
	GPSMisc GPSMisc `desc:"GPS misc additional data"`

	// GPS version of the time
	GPSDate time.Time `desc:"GPS version of the time"`

	// standard exposure info
	Exposure Exposure `desc:"standard exposure info"`

	// full set of name / value tags
	Tags map[string]string `desc:"full set of name / value tags"`

	// full path to thumb file name -- e.g., encoded as a .jpg
	Thumb string `json:"-" view:"-" desc:"full path to thumb file name -- e.g., encoded as a .jpg"`

	// general-purpose flag state, e.g., for pruning old files
	Flagged bool `json:"-" view:"-" desc:"general-purpose flag state, e.g., for pruning old files"`
}

func (pi *Info) Defaults() {
	pi.Depth = 8
}

// FileBase returns the base, no extension file name (used as Key ic PicsMap)
func (pi *Info) FileBase() string {
	fb := filepath.Base(pi.File)
	fnext, _ := dirs.SplitExt(fb)
	return fnext
}

// SetFileThumbFmBase sets the File and Thumb name based on given
// file *base* name (no extension) and File directory, Thumb directory.
// Ext must already have been set.
// This is useful for initializing after loading or renaming base name.
func (pi *Info) SetFileThumbFmBase(fnext, fdir, tdir string) {
	fn := fnext + pi.Ext
	ffn := filepath.Join(fdir, fn)
	tfn := filepath.Join(tdir, fnext+".jpg")
	pi.File = ffn
	pi.Thumb = tfn
}

// SetFileThumbFmFile sets the File and Thumb name based on given
// full File path name and Thumb directory. Sets Ext.
// This is useful for creating new Info from file.
func (pi *Info) SetFileThumbFmFile(fname, tdir string) {
	fb := filepath.Base(pi.File)
	fnext, ext := dirs.SplitExt(fb)
	pi.Ext = ext
	tfn := filepath.Join(tdir, fnext+".jpg")
	pi.File = fname
	pi.Thumb = tfn
}

// DiffsTo returns any differences between this and another Info record
func (pi *Info) DiffsTo(npi *Info) []string {
	var dl []string
	if pi.Ext != npi.Ext {
		dl = append(dl, fmt.Sprintf("Ext differs: %s != %s\n", pi.Ext, npi.Ext))
	}
	if pi.Desc != npi.Desc {
		dl = append(dl, fmt.Sprintf("Desc differs: %s != %s\n", pi.Desc, npi.Desc))
	}
	if pi.FileMod != npi.FileMod {
		dl = append(dl, fmt.Sprintf("FileMod differs: %v != %v\n", pi.FileMod, npi.FileMod))
	}
	if pi.Sup != npi.Sup {
		dl = append(dl, fmt.Sprintf("Sup differs: %v != %v\n", pi.Sup, npi.Sup))
	}
	if pi.Number != npi.Number {
		dl = append(dl, fmt.Sprintf("Number differs: %v != %v\n", pi.Number, npi.Number))
	}
	if pi.Size != npi.Size {
		dl = append(dl, fmt.Sprintf("Size differs: %v != %v\n", pi.Size, npi.Size))
	}
	if pi.Depth != npi.Depth {
		dl = append(dl, fmt.Sprintf("Depth differs: %v != %v\n", pi.Depth, npi.Depth))
	}
	if pi.Orient != npi.Orient {
		dl = append(dl, fmt.Sprintf("Orient differs: %v != %v\n", pi.Orient, npi.Orient))
	}
	if pi.DateTaken != npi.DateTaken {
		dl = append(dl, fmt.Sprintf("DateTaken differs: %v != %v\n", pi.DateTaken, npi.DateTaken))
	}
	if pi.DateMod != npi.DateMod {
		dl = append(dl, fmt.Sprintf("DateMod differs: %v != %v\n", pi.DateMod, npi.DateMod))
	}
	if pi.GPSLoc != npi.GPSLoc {
		dl = append(dl, fmt.Sprintf("GPSLoc differs: %v != %v\n", pi.GPSLoc, npi.GPSLoc))
	}
	if pi.GPSMisc != npi.GPSMisc {
		dl = append(dl, fmt.Sprintf("GPSMisc differs: %v != %v\n", pi.GPSMisc, npi.GPSMisc))
	}
	if pi.GPSDate != npi.GPSDate {
		dl = append(dl, fmt.Sprintf("GPSDate differs: %v != %v\n", pi.GPSDate, npi.GPSDate))
	}
	if pi.Exposure != npi.Exposure {
		dl = append(dl, fmt.Sprintf("Exposure differs: %v != %v\n", pi.Exposure, npi.Exposure))
	}
	return dl
}

///////////////////////////////////////////////////////////////////////////////////
//  Additional Structs

// Orientations are the exif rotations and mirroring codes
type Orientations int

const (
	// NoOrient means no orientation information was set -- assume Rotate0
	NoOrient Orientations = iota

	// Rotated0 means the image data is in the correct orientation as is
	Rotated0

	// FlippedH means the image is flipped in the horizontal axis
	FlippedH

	// Rotated180 means the image is rotated 180 degrees
	Rotated180

	// FlippedV means the image is flipped in the vertical axis
	FlippedV

	// FlippedHRotated90L means the image is flipped horizontally and rotated 90 degrees left
	FlippedHRotated90L

	// Rotated90L means the image is rotated 90 degrees to the left (counter-clockwise)
	Rotated90L

	// FlippedHRotated90R means the image is flipped horizontally and rotated 90 degrees right
	FlippedHRotated90R

	// Rotated90R means the image is rotated 90 degrees to the right (clockwise)
	Rotated90R

	// OrientUndef means undefined
	OrientUndef

	OrientationsN
)

//go:generate stringer -type=Orientations

var KiT_Orientations = kit.Enums.AddEnum(OrientationsN, kit.NotBitFlag, nil)

func (ev Orientations) MarshalJSON() ([]byte, error)  { return kit.EnumMarshalJSON(ev) }
func (ev *Orientations) UnmarshalJSON(b []byte) error { return kit.EnumUnmarshalJSON(ev, b) }

// Rotate returns orientation updated with given +/-90 or 180 degree rotation.
// In general, it goes the opposite of the name as that is what is done to compensate.
func (or Orientations) Rotate(deg int) Orientations {
	switch or {
	case NoOrient, Rotated0, OrientUndef:
		switch deg {
		case 90:
			return Rotated90L
		case -90:
			return Rotated90R
		case 180:
			return Rotated180
		}
	case Rotated90L:
		switch deg {
		case 90:
			return Rotated180
		case -90:
			return Rotated0
		case 180:
			return Rotated90R
		}
	case Rotated90R:
		switch deg {
		case 90:
			return Rotated0
		case -90:
			return Rotated180
		case 180:
			return Rotated90L
		}
	case Rotated180:
		switch deg {
		case 90:
			return Rotated90R
		case -90:
			return Rotated90L
		case 180:
			return Rotated0
		}
	}
	return or
}

// OrientSize returns the size after image is oriented accordingly
func (or Orientations) OrientSize(sz image.Point) image.Point {
	osz := sz
	switch or {
	case Rotated90L, Rotated90R, FlippedHRotated90L, FlippedHRotated90R:
		osz.X, osz.Y = sz.Y, sz.X
	}
	return osz
}

// GPSCoord is a GPS position as decimal degrees
type GPSCoord struct {

	// latitutde as decimal degrees -- a single value in range +/-90.etc
	Lat float64 `desc:"latitutde as decimal degrees -- a single value in range +/-90.etc"`

	// longitude as decimal degrees -- a single value in range +/-180.etc
	Long float64 `desc:"longitude as decimal degrees -- a single value in range +/-180.etc"`

	// altitude in meters
	Alt float64 `desc:"altitude in meters"`
}

// GPSMisc is GPS bearing and other extra data
type GPSMisc struct {

	// destination bearing -- where is the phone going
	DestBearing float64 `desc:"destination bearing -- where is the phone going"`

	// reference for bearing:  M = magnetic, T = true north
	DestBearingRef string `desc:"reference for bearing:  M = magnetic, T = true north"`

	// image direction -- where the phone is pointing
	ImgDir float64 `desc:"image direction -- where the phone is pointing"`

	// reference for image direction: M = magnetic, T = true north
	ImgDirRef string `desc:"reference for image direction: M = magnetic, T = true north"`

	// camera speed
	Speed float64 `desc:"camera speed"`

	// camera speed reference: K = Km/hr, M = MPH, N = knots
	SpeedRef string `desc:"camera speed reference: K = Km/hr, M = MPH, N = knots"`
}

// DecDegFromDMS converts from degrees, minutes and seconds to a decimal
func DecDegFromDMS(degs, mins, secs float64) float64 {
	return degs + mins/60 + secs/3600
}

// Exposure has standard exposure information
type Exposure struct {

	// exposure time
	Time float64 `desc:"exposure time"`

	// fstop
	FStop float64 `desc:"fstop"`

	// ISO speed
	ISOSpeed float64 `desc:"ISO speed"`

	// focal length
	FocalLen float64 `desc:"focal length"`

	// aperture
	Aperture float64 `desc:"aperture"`
}
