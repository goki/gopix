// Copyright (c) 2020, The GoKi Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package main

import (
	"fmt"
	"image"
	"io/ioutil"
	"log"
	"os"
	"sort"
	"time"

	"github.com/dsoprea/go-exif/v2"
	exifcommon "github.com/dsoprea/go-exif/v2/common"
)

// PicInfo is the information about a picture / video, extracted from
// EXIF format etc
type PicInfo struct {
	File      string            `desc:"file name"`
	Thumb     string            `desc:"thumb file"`
	Size      image.Point       `desc:"size of image in raw pixels"`
	DateTaken time.Time         `desc:"date when the image / video was taken"`
	DateMod   time.Time         `desc:"date when image was last modified / edit"`
	GPSLoc    GPSCoord          `desc:"GPS coordinates of location of shot"`
	GPSDate   time.Time         `desc:"GPS version of the time"`
	Exposure  Exposure          `desc:"standard exposure info"`
	Tags      map[string]string `desc:"full set of name / value tags"`
}

// Pics is a slice of PicInfo for all the pictures
type Pics []*PicInfo

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

// GPSCoord is a GPS position as decimal degrees
type GPSCoord struct {
	Lat     float64 `desc:"latitutde as decimal degrees -- a single value in range +/-90.etc"`
	Long    float64 `desc:"longitude as decimal degrees -- a single value in range +/-180.etc"`
	Alt     float64 `desc:"altitude -- units?"`
	Bearing float64 `desc:"bearing -- ??"`
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

// One entry of EXIF data -- used internally
type IfdEntry struct {
	IfdPath     string                      `json:"ifd_path"`
	FqIfdPath   string                      `json:"fq_ifd_path"`
	IfdIndex    int                         `json:"ifd_index"`
	TagId       uint16                      `json:"tag_id"`
	TagName     string                      `json:"tag_name"`
	TagTypeId   exifcommon.TagTypePrimitive `json:"tag_type_id"`
	TagTypeName string                      `json:"tag_type_name"`
	UnitCount   uint32                      `json:"unit_count"`
	Value       interface{}                 `json:"value"`
	ValueString string                      `json:"value_string"`
}

func (e *IfdEntry) ToInt() int {
	switch e.TagTypeId {
	case exifcommon.TypeLong:
		vl := e.Value.([]uint32)
		return int(vl[0])
	case exifcommon.TypeShort:
		vl := e.Value.([]uint16)
		return int(vl[0])
	case exifcommon.TypeSignedLong:
		vl := e.Value.([]int32)
		return int(vl[0])
	case exifcommon.TypeRational:
		vl := e.Value.([]exifcommon.Rational)
		den := int(vl[0].Denominator)
		if den != 0 {
			return int(vl[0].Numerator) / den
		}
		return 0
	case exifcommon.TypeSignedRational:
		vl := e.Value.([]exifcommon.SignedRational)
		den := int(vl[0].Denominator)
		if den != 0 {
			return int(vl[0].Numerator) / den
		}
		return 0
	}
	return 0
}

func (e *IfdEntry) ToFloat() float64 {
	switch e.TagTypeId {
	case exifcommon.TypeLong:
		vl := e.Value.([]uint32)
		return float64(vl[0])
	case exifcommon.TypeShort:
		vl := e.Value.([]uint16)
		return float64(vl[0])
	case exifcommon.TypeSignedLong:
		vl := e.Value.([]int32)
		return float64(vl[0])
	case exifcommon.TypeRational:
		vl := e.Value.([]exifcommon.Rational)
		den := float64(vl[0].Denominator)
		if den != 0 {
			return float64(vl[0].Numerator) / den
		}
		return 0
	case exifcommon.TypeSignedRational:
		vl := e.Value.([]exifcommon.SignedRational)
		den := float64(vl[0].Denominator)
		if den != 0 {
			return float64(vl[0].Numerator) / den
		}
		return 0
	}
	return 0
}

func (e *IfdEntry) ToFloats() []float64 {
	rf := make([]float64, e.UnitCount)
	switch e.TagTypeId {
	case exifcommon.TypeLong:
		vl := e.Value.([]uint32)
		for i := range vl {
			rf[i] = float64(vl[i])
		}
	case exifcommon.TypeShort:
		vl := e.Value.([]uint16)
		for i := range vl {
			rf[i] = float64(vl[i])
		}
	case exifcommon.TypeSignedLong:
		vl := e.Value.([]int32)
		for i := range vl {
			rf[i] = float64(vl[i])
		}
	case exifcommon.TypeRational:
		vl := e.Value.([]exifcommon.Rational)
		for i := range vl {
			den := float64(vl[i].Denominator)
			if den != 0 {
				rf[i] = float64(vl[i].Numerator) / den
			}
		}
	case exifcommon.TypeSignedRational:
		vl := e.Value.([]exifcommon.SignedRational)
		for i := range vl {
			den := float64(vl[i].Denominator)
			if den != 0 {
				rf[i] = float64(vl[i].Numerator) / den
			}
		}
	}
	return rf
}

func ReadExif(fn string) (*PicInfo, error) {
	pi := &PicInfo{File: fn}

	f, err := os.Open(fn)
	defer f.Close()

	if err != nil {
		log.Println(err)
		return pi, err
	}
	data, err := ioutil.ReadAll(f)
	if err != nil {
		log.Println(err)
		return pi, err
	}
	rawExif, err := exif.SearchAndExtractExif(data)
	if err != nil {
		if err == exif.ErrNoExif {
			// log.Println(err)
			return pi, err
		}
		log.Println(err)
		return pi, err
	}

	// Run the parse.
	im := exif.NewIfdMappingWithStandard()
	ti := exif.NewTagIndex()

	entries := make([]IfdEntry, 0)
	visitor := func(fqIfdPath string, ifdIndex int, ite *exif.IfdTagEntry) (err error) {
		tagId := ite.TagId()
		tagType := ite.TagType()
		ifdPath, err := im.StripPathPhraseIndices(fqIfdPath)
		if err != nil {
			return err
		}

		it, err := ti.Get(ifdPath, tagId)
		if err != nil {
			if err == exif.ErrTagNotFound {
				fmt.Printf("WARNING: Unknown tag: [%s] (%04x)\n", ifdPath, tagId)
				return nil
			} else {
				return err
			}
		}
		value, err := ite.Value()
		if err != nil {
			if err == exifcommon.ErrUnhandledUndefinedTypedTag {
				// fmt.Printf("WARNING: Non-standard undefined tag: [%s] (%04x)\n", ifdPath, tagId)
				return nil
			}
			return err
		}
		valueString, err := ite.FormatFirst()
		entry := IfdEntry{
			IfdPath:     ifdPath,
			FqIfdPath:   fqIfdPath,
			IfdIndex:    ifdIndex,
			TagId:       tagId,
			TagName:     it.Name,
			TagTypeId:   tagType,
			TagTypeName: tagType.String(),
			UnitCount:   ite.UnitCount(),
			Value:       value,
			ValueString: valueString,
		}
		entries = append(entries, entry)
		return nil
	}
	_, err = exif.Visit(exifcommon.IfdStandard, im, ti, rawExif, visitor)
	lat := [4]float64{}
	long := [4]float64{}
	pi.Tags = make(map[string]string)
	for _, e := range entries {
		// fmt.Printf("Tag: %s  Value: %s\n", e.TagName, e.ValueString)
		switch e.TagName {
		case "DateTimeOriginal":
			pi.DateTaken, err = time.Parse("2006:01:02 15:04:05", e.ValueString)
			if err != nil {
				log.Println(err)
			}
		case "DateTimeDigitized":
			pi.DateTaken, err = time.Parse("2006:01:02 15:04:05", e.ValueString)
			if err != nil {
				log.Println(err)
			}
		case "PixelYDimension":
			pi.Size.Y = e.ToInt()
		case "PixelXDimension":
			pi.Size.X = e.ToInt()
		case "ExposureTime":
			pi.Exposure.Time = e.ToFloat()
		case "ISOSpeedRatings":
			pi.Exposure.ISOSpeed = e.ToFloat()
		case "ApertureValue":
			pi.Exposure.Aperture = e.ToFloat()
		case "FocalLength":
			pi.Exposure.FocalLen = e.ToFloat()
		case "FNumber":
			pi.Exposure.FStop = e.ToFloat()
		case "GPSLatitudeRef":
			if e.ValueString == "N" {
				lat[3] = 1
			} else {
				lat[3] = -1
			}
		case "GPSLatitude":
			rf := e.ToFloats()
			for i := range rf {
				lat[i] = rf[i]
			}
		case "GPSLongitudeRef":
			if e.ValueString == "E" {
				long[3] = 1
			} else {
				long[3] = -1
			}
		case "GPSLongitude":
			rf := e.ToFloats()
			for i := range rf {
				long[i] = rf[i]
			}
		case "GPSAltitude":
			pi.GPSLoc.Alt = e.ToFloat()
		case "GPSBearing":
			pi.GPSLoc.Bearing = e.ToFloat()
		case "GPSDateStamp":
			pi.GPSDate, err = time.Parse("2006:01:02", e.ValueString)
			if err != nil {
				log.Println(err)
			}
		case "MakerNote":
		case "UserComment":
		case "ComponentsConfiguration":
		default:
			pi.Tags[e.TagName] = e.ValueString
		}
	}
	if lat[3] != 0 {
		lat[0] *= lat[3]
	}
	if long[3] != 0 {
		long[0] *= long[3]
	}
	pi.GPSLoc.Lat = DecDegFromDMS(lat[0], lat[1], lat[2])
	pi.GPSLoc.Long = DecDegFromDMS(long[0], long[1], long[2])
	if pi.DateMod.IsZero() {
		pi.DateMod = pi.DateTaken
	}
	return pi, nil
}
