// Copyright (c) 2020, The GoKi Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package picinfo

import (
	"image"
	"log"
	"os"

	"github.com/anthonynsimon/bild/imgio"
	"github.com/anthonynsimon/bild/transform"
	"github.com/goki/pi/filecat"
	"github.com/jdeng/goheif"
)

// OpenImage handles all image formats including jpeg or HEIC
func OpenImage(fnm string) (image.Image, error) {
	typ := filecat.SupportedFromFile(fnm)
	// todo: deal with movies?
	var img image.Image
	var err error
	switch typ {
	case filecat.Heic:
		img, err = OpenHEIC(fnm)
	default:
		img, err = imgio.Open(fnm)
	}
	if err != nil {
		log.Println(err)
	}
	return img, err
}

// OpenHEIC opens a HEIC formatted file
func OpenHEIC(fnm string) (image.Image, error) {
	f, err := os.Open(fnm)
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
