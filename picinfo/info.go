// Copyright (c) 2020, The GoKi Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package picinfo

import (
	"image"
	"time"

	"github.com/goki/ki/kit"
	"github.com/goki/pi/filecat"
)

// Info is the information about a picture / video file
type Info struct {
	File      string            `json:"-" desc:"full path to image file name"`
	Desc      string            `desc:"image description -- can contain arbitrary user comments -- ascii encoded"`
	FileMod   time.Time         `desc:"date when image file was modified"`
	Sup       filecat.Supported `desc:"supported type of image file, decoded from extension, using gopi/filecat system"`
	Number    int               `desc:"if there are multiple files taken at the same time, e.g., in a Burst, this is the number"`
	Size      image.Point       `desc:"size of image in raw pixels"`
	Depth     int               `desc:"number of bits in each color component (e.g., 8 is typical)"`
	Orient    Orientations      `desc:"orientation of the image using exif standards that include rotation and mirroring"`
	DateTaken time.Time         `desc:"date when the image / video was taken"`
	DateMod   time.Time         `desc:"date when image was last modified / edited"`
	GPSLoc    GPSCoord          `desc:"GPS coordinates of location of shot"`
	GPSDate   time.Time         `desc:"GPS version of the time"`
	Exposure  Exposure          `desc:"standard exposure info"`
	Tags      map[string]string `desc:"full set of name / value tags"`
	Thumb     string            `json:"-" view:"-" desc:"full path to thumb file name -- e.g., encoded as a .jpg"`
}

func (inf *Info) Defaults() {
	inf.Depth = 8
}

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
	Lat            float64 `desc:"latitutde as decimal degrees -- a single value in range +/-90.etc"`
	Long           float64 `desc:"longitude as decimal degrees -- a single value in range +/-180.etc"`
	Alt            float64 `desc:"altitude in meters"`
	DestBearing    float64 `desc:"destination bearing -- where is the phone going"`
	DestBearingRef string  `desc:"reference for bearing:  M = magnetic, T = true north"`
	ImgDir         float64 `desc:"image direction -- where the phone is pointing"`
	ImgDirRef      string  `desc:"reference for image direction: M = magnetic, T = true north"`
	Speed          float64 `desc:"camera speed"`
	SpeedRef       string  `desc:"camera speed reference: K = Km/hr, M = MPH, N = knots"`
}

// DecDegFromDMS converts from degrees, minutes and seconds to a decimal
func DecDegFromDMS(degs, mins, secs float64) float64 {
	return degs + mins/60 + secs/3600
}

// Exposure has standard exposure information
type Exposure struct {
	Time     float64 `desc:"exposure time"`
	FStop    float64 `desc:"fstop"`
	ISOSpeed float64 `desc:"ISO speed"`
	FocalLen float64 `desc:"focal length"`
	Aperture float64 `desc:"aperture"`
}
