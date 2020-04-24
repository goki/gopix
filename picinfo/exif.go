// Copyright (c) 2020, The GoKi Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package picinfo

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"image"
	"image/jpeg"
	"io"
	"log"
	"os"
	"path/filepath"
	"time"

	"github.com/dsoprea/go-exif/v2"
	exifcommon "github.com/dsoprea/go-exif/v2/common"
	jpegstructure "github.com/dsoprea/go-jpeg-image-structure"
	"github.com/goki/pi/filecat"
	"github.com/jdeng/goheif"
)

// reference for all defined tags:
// https://www.exiv2.org/tags.html

// todo: support exif for other filetypes:
// PNG: https://stackoverflow.com/questions/9542359/does-png-contain-exif-data-like-jpg
// TIFF: this is a basic tiff thing -- but std go package does not support exif:
// https://godoc.org/golang.org/x/image/tiff

// OpenNewInfo opens file and reads the exif info for given file, returning
// a new Info with that info all set.
func OpenNewInfo(fn string) (*Info, error) {
	rawExif, err := OpenRawExif(fn)
	if err != nil && err != exif.ErrNoExif {
		log.Println(err)
		return nil, err
	}
	pi, err := NewInfoForFile(fn)
	if err != nil {
		log.Println(err)
		return nil, err
	}
	pi.ParseRawExif(rawExif)
	return pi, err
}

// NewInfoForFile returns a new Info initialized with basic info from file
func NewInfoForFile(fn string) (*Info, error) {
	pi := &Info{File: fn}
	pi.Defaults()
	pi.Sup = filecat.SupportedFromFile(fn)
	fst, err := os.Stat(fn)
	if err == nil {
		pi.FileMod = fst.ModTime()
	}
	pi.DateTaken = pi.FileMod // method of last resort
	pi.DateMod = pi.FileMod
	return pi, err
}

// OpenRawExif opens the raw exif data bytes from given file.
// if Jpeg or Heic, it returns the exact correct raw bytes --
// otherwise it is the *entire file* and not suitable for re-writing.
func OpenRawExif(fn string) ([]byte, error) {
	sup := filecat.SupportedFromFile(fn)
	switch sup {
	case filecat.Heic:
		f, err := os.Open(fn)
		defer f.Close()
		if err != nil {
			return nil, err
		}
		return goheif.ExtractExif(f)
	case filecat.Jpeg:
		data, err := OpenBytes(fn)
		if err != nil {
			return nil, err
		}
		jmp := jpegstructure.NewJpegMediaParser()
		intfc, err := jmp.ParseBytes(data)
		if err != nil {
			log.Println(err)
			return nil, err
		}
		sl := intfc.(*jpegstructure.SegmentList)
		_, s, err := sl.FindExif()
		if err == exif.ErrNoExif {
			return nil, nil
		}
		if err != nil {
			log.Println(err)
			return nil, err
		}
		_, rawExif, err := s.Exif()
		if err != nil {
			log.Println(err)
			return nil, err
		}
		return rawExif, err
	default:
		data, err := OpenBytes(fn)
		if err != nil {
			return nil, err
		}
		return exif.SearchAndExtractExif(data)
	}
}

// ParseRawExif parses the raw Exif data into our Info structure
func (pi *Info) ParseRawExif(rawExif []byte) {
	if rawExif == nil {
		return
	}
	fnbase := filepath.Base(pi.File)

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
	_, err := exif.Visit(exifcommon.IfdStandard, im, ti, rawExif, visitor)
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
				log.Printf("File: %s err: %v\n", fnbase, err)
				dto = time.Time{}
			}
		case "DateTimeDigitized":
			dtd, err = time.Parse("2006:01:02 15:04:05", e.ValueString)
			if err != nil {
				log.Printf("File: %s err: %v\n", fnbase, err)
				dtd = time.Time{}
			}
		case "DateTime":
			dtp, err = time.Parse("2006:01:02 15:04:05", e.ValueString)
			if err != nil {
				log.Printf("File: %s err: %v\n", fnbase, err)
				dtp = time.Time{}
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
		case "ImageDescription":
			pi.Desc = e.ValueString
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
			gpd, err := time.Parse("2006:01:02", e.ValueString)
			if err != nil {
				log.Printf("File: %s err: %v\n", fnbase, err)
			} else {
				pi.GPSDate = gpd
			}
		case "GPSTimeStamp":
			gpstime = e.ToFloats()
		case "ComponentsConfiguration":
		case "UserComment": // usu not useful and long.
		case "MakerNote":
		case "InteroperabilityIndex":
		case "InteroperabilityVersion":
		case "ExifTag":
		case "ExifVersion":
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
	} else {
		pi.DateMod = pi.DateTaken
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
	if gpstime != nil {
		durf := gpstime[0]*3600 + gpstime[1]*60 + gpstime[2]
		pi.GPSDate.Add(time.Second * time.Duration(durf))
	}
}

// UpdateExif reads the exif from file, and generates a new exif incorporating
// changes from given Info.  if rootIfd != nil it is used as a starting point
// otherwise it is generated from the rawExif, which also can be nil if starting fresh.
// returns true if data was different and requires saving.
func (pi *Info) UpdateExif(rawExif []byte, rootIfd *exif.Ifd) (*exif.IfdBuilder, bool, error) {
	ci, err := NewInfoForFile(pi.File)
	ci.ParseRawExif(rawExif)

	if rootIfd == nil && rawExif != nil {
		im := exif.NewIfdMappingWithStandard()
		ti := exif.NewTagIndex()
		_, index, err := exif.Collect(im, ti, rawExif)
		if err != nil {
			return nil, false, err
		}
		rootIfd = index.RootIfd
	}

	var ib *exif.IfdBuilder
	if rootIfd != nil {
		ib = exif.NewIfdBuilderFromExistingChain(rootIfd)
	} else {
		im := exif.NewIfdMappingWithStandard()
		ti := exif.NewTagIndex()
		ib = exif.NewIfdBuilder(im, ti, exifcommon.IfdPathStandard, binary.BigEndian)
	}

	ifchld, err := exif.GetOrCreateIbFromRootIb(ib, "IFD")
	if err != nil {
		log.Printf("create path %s err: %s\n", "IFD", err)
	}
	exchld, err := exif.GetOrCreateIbFromRootIb(ib, "IFD/Exif")
	if err != nil {
		log.Printf("create path %s err: %s\n", "IFD/Exif", err)
	}

	updt := false
	if ci.DateTaken != pi.DateTaken {
		err = ifchld.SetStandardWithName("DateTimeOriginal", exif.ExifFullTimestampString(pi.DateTaken))
		if err != nil {
			log.Printf("date set err: %s\n", err)
		}
		updt = true
	}
	if ci.Number != pi.Number {
		err = ifchld.SetStandardWithName("ImageNumber", intToLong(pi.Number))
		if err != nil {
			log.Printf("number set err: %s\n", err)
		}
		updt = true
	}
	if ci.Size.Y != pi.Size.Y {
		err = exchld.SetStandardWithName("PixelYDimension", intToLong(pi.Size.Y))
		if err != nil {
			log.Printf("pix set err: %s\n", err)
		}
		updt = true
	}
	if ci.Size.X != pi.Size.X {
		err = exchld.SetStandardWithName("PixelXDimension", intToLong(pi.Size.X))
		if err != nil {
			log.Printf("pix set err: %s\n", err)
		}
		updt = true
	}
	if ci.Orient != pi.Orient {
		err = ifchld.SetStandardWithName("Orientation", intToShort(int(pi.Orient)))
		if err != nil {
			log.Printf("orient set err: %s\n", err)
		}
		updt = true
	}
	if ci.Desc != pi.Desc {
		err = ifchld.SetStandardWithName("ImageDescription", pi.Desc)
		if err != nil {
			log.Printf("desc set err: %s\n", err)
		}
		updt = true
	}
	// if ci.GPSLoc.Lat != pi.GPSLoc.Lat {
	// 	childIb.SetStandardWithName("Orientation", uint16(pi.Orient))
	// 	updt = true
	// }
	//
	if updt {
		pi.DateMod = time.Now()
		err = ifchld.SetStandardWithName("DateTime", exif.ExifFullTimestampString(pi.DateMod))
		if err != nil {
			log.Printf("datetime set err: %s\n", err)
		}
	}
	return ib, updt, err
}

// UpdateFileMod updates the modification time on the file
func (pi *Info) UpdateFileMod() error {
	fst, err := os.Stat(pi.File)
	if err == nil {
		pi.FileMod = fst.ModTime()
	}
	return err
}

// SaveJpegUpdated saves a new Jpeg encoded file with info updated to reflect given info
func (pi *Info) SaveJpegUpdated() error {
	data, err := OpenBytes(pi.File)
	if err != nil {
		log.Println(err)
		return err
	}
	jmp := jpegstructure.NewJpegMediaParser()
	intfc, err := jmp.ParseBytes(data)
	if err != nil {
		log.Println(err)
		return err
	}
	sl := intfc.(*jpegstructure.SegmentList)
	_, s, err := sl.FindExif()
	if err != nil && err != exif.ErrNoExif {
		log.Println(err)
		return err
	}
	var rootIfd *exif.Ifd
	var rawExif []byte
	if s != nil {
		rootIfd, rawExif, err = s.Exif()
		if err != nil {
			log.Println(err)
			return err
		}
	}
	if pi.Size == image.ZP {
		img, err := jpeg.Decode(bytes.NewBuffer(data))
		if err == nil {
			pi.Size = img.Bounds().Size()
		}
	}

	ib, updt, err := pi.UpdateExif(rawExif, rootIfd)
	if err != nil {
		log.Println(err)
		return err
	}
	if !updt {
		fmt.Printf("File: %s had no updates to Exif data\n", pi.File)
		return nil
	}
	sl.SetExif(ib)
	if err != nil {
		log.Println(err)
		return err
	}

	f, err := os.Create(pi.File)
	if err != nil {
		log.Println(err)
		return err
	}

	err = sl.Write(f)
	f.Close()
	pi.UpdateFileMod()
	return nil
}

// SaveJpegNew saves a new Jpeg encoded file with exif data generated from current info
func (pi *Info) SaveJpegNew(img image.Image) error {
	ib, _, err := pi.UpdateExif(nil, nil)
	if err != nil {
		log.Println(err)
		return err
	}

	ibe := exif.NewIfdByteEncoder()
	exifData, err := ibe.EncodeToExif(ib)
	if err != nil {
		log.Println(err)
		return err
	}

	return pi.SaveJpegExif(exifData, img)
}

// AddExifPrefix adds the standard Exif00 prefix to given encoded exif data
// if not already present
func AddExifPrefix(exifData []byte) []byte {
	pfx := jpegstructure.ExifPrefix
	pl := len(pfx)
	if len(exifData) >= pl && bytes.Equal(exifData[:pl], pfx) {
		return exifData
	}
	rawExif := make([]byte, pl+len(exifData))
	copy(rawExif, pfx)
	copy(rawExif[pl:], exifData)
	return rawExif
}

// SaveJpegUpdatedExif saves a new Jpeg encoded file with given raw bytes of exif data,
// which is updated to current Info settings prior to saving.
func (pi *Info) SaveJpegUpdatedExif(rawExif []byte, img image.Image) error {
	ib, _, err := pi.UpdateExif(rawExif, nil)
	ibe := exif.NewIfdByteEncoder()
	exifData, err := ibe.EncodeToExif(ib)
	if err != nil {
		log.Println(err)
		return err
	}
	return pi.SaveJpegExif(exifData, img)
}

// SaveJpegExif saves a new Jpeg encoded file with given raw bytes of exif data
// Note: rawExif does NOT have to have the standard Exif00 prefix already -- will be added
func (pi *Info) SaveJpegExif(rawExif []byte, img image.Image) error {
	f, err := os.Create(pi.File)
	if err != nil {
		log.Println(err)
		return err
	}
	defer f.Close()

	rawExif = AddExifPrefix(rawExif)

	w, _ := newWriterExif(f, rawExif)
	err = jpeg.Encode(w, img, &jpeg.Options{Quality: JpegEncodeQuality})
	if err != nil {
		log.Println(err)
		return err
	}
	return nil
}

///////////////////////////////////////////////////////////////////////////////
//  Utilities

func intToLong(val int) []uint32 {
	return []uint32{uint32(val)}
}

func intToShort(val int) []uint16 {
	return []uint16{uint16(val)}
}

// Skip Writer for exif writing -- used to skip over the 2 byte magic number
// that the default jpeg.Encode will try to write to the file, so we can
// append our own magic number at the start..
// from github.com/jdeng/goheif/heic2jpg
type writerSkipper struct {
	w           io.Writer
	bytesToSkip int
}

func (w *writerSkipper) Write(data []byte) (int, error) {
	if w.bytesToSkip <= 0 {
		return w.w.Write(data)
	}

	if dataLen := len(data); dataLen < w.bytesToSkip {
		w.bytesToSkip -= dataLen
		return dataLen, nil
	}

	if n, err := w.w.Write(data[w.bytesToSkip:]); err == nil {
		n += w.bytesToSkip
		w.bytesToSkip = 0
		return n, nil
	} else {
		return n, err
	}
}

func newWriterExif(w io.Writer, exif []byte) (io.Writer, error) {
	writer := &writerSkipper{w, 2}
	soi := []byte{0xff, 0xd8}
	if _, err := w.Write(soi); err != nil {
		return nil, err
	}

	if exif != nil {
		app1Marker := 0xe1
		marker := []byte{0xff, uint8(app1Marker)}
		if _, err := w.Write(marker); err != nil {
			return nil, err
		}
		len_ := uint16(len(exif) + 2)
		binary.Write(w, binary.BigEndian, &len_)

		if _, err := w.Write(exif); err != nil {
			return nil, err
		}
	}

	return writer, nil
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
