// Copyright (c) 2020, The Goki Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package picinfo

import (
	"fmt"
	"image"
	"image/gif"
	"image/jpeg"
	"image/png"
	"io/ioutil"
	"log"
	"os"

	"github.com/adrium/goheif"
	"github.com/anthonynsimon/bild/transform"
	"github.com/goki/pi/filecat"
	"github.com/spakin/netpbm"
	"golang.org/x/image/bmp"
	"golang.org/x/image/tiff"
)

// JpegEncodeQuality is the default encoding quality for Jpeg files
var JpegEncodeQuality = 90

// GetSize returns the "raw" size of the image, either from info value
// or directly from image -- this is the non-re-oriented size.
// In general it is a good idea to ensure that images have their sizes saved!
func (pi *Info) GetSize() image.Point {
	if pi.Size != image.ZP {
		return pi.Size
	}
	img, err := OpenImage(pi.File)
	if err != nil {
		log.Println(err)
		return image.ZP
	}
	pi.Size = img.Bounds().Size()
	return pi.Size
}

// GetSizeOrient is the size of the image after it is transformed by the Orientation
// i.e., the actual display size of the image.
func (pi *Info) GetSizeOrient() image.Point {
	sz := pi.GetSize()
	return pi.Orient.OrientSize(sz)
}

// ImageOriented returns the opened image for this file,
// oriented according to the Orient setting.
// errors are logged.
func (pi *Info) ImageOriented() (image.Image, error) {
	img, err := OpenImage(pi.File)
	if err != nil {
		log.Println(err)
		return img, err
	}
	img = OrientImage(img, pi.Orient)
	return img, nil
}

// OpenBytes opens file and returns bytes
func OpenBytes(fn string) ([]byte, error) {
	f, err := os.Open(fn)
	defer f.Close()
	if err != nil {
		return nil, err
	}
	return ioutil.ReadAll(f)
}

// OpenImage opens an image from given filename.
// Supports: png, jpeg, tiff, gif, bmp, pgm, pbm, ppm, pnm, and heic formats.
func OpenImage(fname string) (image.Image, error) {
	typ := filecat.SupportedFromFile(fname)
	// todo: deal with movies?
	var img image.Image
	var err error
	switch typ {
	case filecat.Heic:
		img, err = OpenHEIC(fname)
	default:
		img, err = OpenImageAuto(fname)
	}
	if err != nil {
		log.Printf("File: %s  picinfo.OpenImage Error: %v\n", fname, err)
	}
	return img, err
}

// OpenImageAuto opens an image from given filename.
// Format is inferred automatically, using image package decoders registered.
// Supports: png, jpeg, tiff, gif, bmp, pgm, pbm, ppm, pnm formats.
func OpenImageAuto(fname string) (image.Image, error) {
	file, err := os.Open(fname)
	if err != nil {
		return nil, err
	}
	defer file.Close()
	im, _, err := image.Decode(file)
	return im, err
}

// SaveImage saves image to file, with format inferred from filename.
// Supports: png, jpeg, tiff, gif, bmp, pgm, pbm, ppm, pnm formats.
// Uses standard default options -- use encoder for other options.
func SaveImage(fname string, im image.Image) error {
	file, err := os.Create(fname)
	if err != nil {
		return err
	}
	defer file.Close()
	typ := filecat.SupportedFromFile(fname)
	switch typ {
	case filecat.Png:
		return png.Encode(file, im)
	case filecat.Jpeg:
		return jpeg.Encode(file, im, &jpeg.Options{Quality: JpegEncodeQuality})
	case filecat.Tiff:
		return tiff.Encode(file, im, &tiff.Options{Compression: tiff.Deflate}) // Deflate = ZIP = best
	case filecat.Gif:
		return gif.Encode(file, im, nil)
	case filecat.Bmp:
		return bmp.Encode(file, im)
	case filecat.Pgm:
		return netpbm.Encode(file, im, &netpbm.EncodeOptions{Format: netpbm.PGM})
	case filecat.Pbm:
		return netpbm.Encode(file, im, &netpbm.EncodeOptions{Format: netpbm.PBM})
	case filecat.Ppm:
		return netpbm.Encode(file, im, &netpbm.EncodeOptions{Format: netpbm.PPM})
	case filecat.Pnm:
		return netpbm.Encode(file, im, &netpbm.EncodeOptions{Format: netpbm.PNM})
	default:
		return fmt.Errorf("picinfo.SaveImage: file type: %s not supported", typ.String())
	}
}

// OpenHEIC opens a HEIC formatted file
func OpenHEIC(fname string) (image.Image, error) {
	f, err := os.Open(fname)
	if err != nil {
		return nil, err
	}
	defer f.Close()

	img, err := goheif.Decode(f)
	if err != nil {
		return nil, err
	}

	return img, nil
}

// OrientImage returns an image with the proper orientation as specified
func OrientImage(img image.Image, orient Orientations) image.Image {
	if orient <= Rotated0 || orient >= OrientUndef {
		return img
	}
	opts := &transform.RotationOptions{ResizeBounds: true}
	switch orient {
	case FlippedH:
		return transform.FlipH(img)
	case Rotated180:
		return transform.Rotate(img, 180, opts)
	case FlippedV:
		return transform.FlipV(img)
	case FlippedHRotated90L:
		return transform.Rotate(transform.FlipH(img), 90, opts)
	case Rotated90L:
		return transform.Rotate(img, 90, opts)
	case FlippedHRotated90R:
		return transform.Rotate(transform.FlipH(img), -90, opts)
	case Rotated90R:
		return transform.Rotate(img, -90, opts)
	}
	return img
}
