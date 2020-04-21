// Copyright (c) 2020, The GoKi Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package picinfo

import (
	"fmt"
	"io/ioutil"
	"log"
	"os"
	"time"

	"github.com/dsoprea/go-exif/v2"
	exifcommon "github.com/dsoprea/go-exif/v2/common"
	"github.com/goki/pi/filecat"
)

// reference for all defined tags:
// https://www.exiv2.org/tags.html

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

// ReadExif reads the exif info for given file, which should be full path to file
func ReadExif(fn string) (*Info, error) {
	pi := &Info{File: fn}
	pi.Defaults()

	pi.Sup = filecat.SupportedFromFile(fn)

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
	var gpstime []float64
	pi.Tags = make(map[string]string)
	var dto time.Time
	var dtd time.Time
	var dtp time.Time
	for _, e := range entries {
		// fmt.Printf("Tag: %s  Value: %s\n", e.TagName, e.ValueString)
		switch e.TagName {
		case "DateTimeOriginal":
			dto, err = time.Parse("2006:01:02 15:04:05", e.ValueString)
			if err != nil {
				log.Println(err)
			}
		case "DateTimeDigitized":
			dtd, err = time.Parse("2006:01:02 15:04:05", e.ValueString)
			if err != nil {
				log.Println(err)
			}
		case "DateTime":
			dtp, err = time.Parse("2006:01:02 15:04:05", e.ValueString)
			if err != nil {
				log.Println(err)
			}
		case "ImageNumber":
			pi.Number = e.ToInt()
		case "PixelYDimension":
			pi.Size.Y = e.ToInt()
		case "PixelXDimension":
			pi.Size.X = e.ToInt()
		case "BitsPerSample":
			pi.Depth = e.ToInt()
		case "Orientation":
			pi.Orient = Orientations(e.ToInt())
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
		case "GPSAltitudeRef":
		case "GPSBearing":
			pi.GPSLoc.DestBearing = e.ToFloat()
		case "GPSDestBearing":
			pi.GPSLoc.DestBearing = e.ToFloat()
		case "GPSDestBearingRef":
			pi.GPSLoc.DestBearingRef = e.ValueString
		case "GPSImgDirection":
			pi.GPSLoc.ImgDir = e.ToFloat()
		case "GPSImgDirectionRef":
			pi.GPSLoc.ImgDirRef = e.ValueString
		case "GPSSpeed":
			pi.GPSLoc.Speed = e.ToFloat()
		case "GPSSpeedRef":
			pi.GPSLoc.SpeedRef = e.ValueString
		case "GPSDateStamp":
			pi.GPSDate, err = time.Parse("2006:01:02", e.ValueString)
			if err != nil {
				log.Println(err)
			}
		case "GPSTimeStamp":
			gpstime = e.ToFloats()
		case "MakerNote":
		case "UserComment":
		case "ComponentsConfiguration":
		default:
			pi.Tags[e.TagName] = e.ValueString
		}
	}
	if !dto.IsZero() {
		pi.DateTaken = dto
	} else if !dtd.IsZero() {
		pi.DateTaken = dtd
	} else if !dtp.IsZero() {
		pi.DateTaken = dtp
	}
	if !dtp.IsZero() && pi.DateTaken != dtp {
		pi.DateMod = dtp
	}
	if lat[3] != 0 {
		lat[0] *= lat[3]
		lat[1] *= lat[3]
		lat[2] *= lat[3]
	}
	if long[3] != 0 {
		long[0] *= long[3]
		long[1] *= long[3]
		long[2] *= long[3]
	}
	pi.GPSLoc.Lat = DecDegFromDMS(lat[0], lat[1], lat[2])
	pi.GPSLoc.Long = DecDegFromDMS(long[0], long[1], long[2])
	if pi.DateMod.IsZero() {
		pi.DateMod = pi.DateTaken
	}
	if gpstime != nil {
		durf := gpstime[0]*3600 + gpstime[1]*60 + gpstime[2]
		pi.GPSDate.Add(time.Duration(durf))
	}
	return pi, nil
}
