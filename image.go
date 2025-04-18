/*
Copyright 2017 Luke Granger-Brown
Modified for use with HugeSpaceship by Henry Asbridge in 2024

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

     http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

// Package dds provides a decoder for the DirectDraw surface format,
// which is compatible with the standard image package.
//
// It should normally be used by importing it with a blank name, which
// will cause it to register itself with the image package:
//
//	import _ "github.com/hugespaceship/dds"
package dds

import (
	"fmt"
	"github.com/mauserzjeh/dxt"
	"image"
	"image/color"
	"io"
)

func init() {
	image.RegisterFormat("dds", "DDS ", Decode, DecodeConfig)
}

func DecodeConfig(r io.Reader) (image.Config, error) {
	h, err := readHeader(r)
	if err != nil {
		return image.Config{}, err
	}

	// set width and height
	c := image.Config{
		Width:  int(h.width),
		Height: int(h.height),
	}

	pf := h.pixelFormat
	hasAlpha := (pf.flags&pfAlphaPixels == pfAlphaPixels) || (pf.flags&pfAlpha == pfAlpha)
	hasRGB := (pf.flags&pfFourCC == pfFourCC) || (pf.flags&pfRGB == pfRGB)
	hasYUV := pf.flags&pfYUV == pfYUV
	hasLuminance := pf.flags&pfLuminance == pfLuminance
	switch {
	case hasRGB && pf.rgbBitCount == 32:
		c.ColorModel = color.RGBAModel
	case hasRGB && pf.rgbBitCount == 64:
		c.ColorModel = color.RGBA64Model
	case hasYUV && pf.rgbBitCount == 24:
		c.ColorModel = color.YCbCrModel
	case hasLuminance && pf.rgbBitCount == 8:
		c.ColorModel = color.GrayModel
	case hasLuminance && pf.rgbBitCount == 16:
		c.ColorModel = color.Gray16Model
	case hasAlpha && pf.rgbBitCount == 8:
		c.ColorModel = color.AlphaModel
	case hasAlpha && pf.rgbBitCount == 16:
		c.ColorModel = color.AlphaModel
	default:
		return image.Config{}, fmt.Errorf("unrecognized image format: hasAlpha: %v, hasRGB: %v, hasYUV: %v, hasLuminance: %v, pf.flags: %x", hasAlpha, hasRGB, hasYUV, hasLuminance, pf.flags)
	}

	return c, nil
}

type img struct {
	h   header
	buf []byte

	rBit, gBit, bBit, aBit uint

	stride, pitch int
}

func (i *img) ColorModel() color.Model {
	return color.NRGBAModel
}

func (i *img) Bounds() image.Rectangle {
	return image.Rect(0, 0, int(i.h.width), int(i.h.height))
}

func (i *img) At(x, y int) color.Color {
	arrPsn := i.pitch*y + i.stride*x
	d := readBits(i.buf[arrPsn:], i.h.pixelFormat.rgbBitCount)
	r := uint8((d & i.h.pixelFormat.rBitMask) >> i.rBit)
	g := uint8((d & i.h.pixelFormat.gBitMask) >> i.gBit)
	b := uint8((d & i.h.pixelFormat.bBitMask) >> i.bBit)
	a := uint8((d & i.h.pixelFormat.aBitMask) >> i.aBit)
	return color.NRGBA{R: r, G: g, B: b, A: a}
}

func Decode(r io.Reader) (image.Image, error) {
	h, err := readHeader(r)
	if err != nil {
		return nil, err
	}

	switch h.pixelFormat.fourCC {
	case compressionTypeNone:
		return decodeUncompressedDDS(h, r)
	case compressionTypeDXT1:
		return decodeDXT1DDS(h, r)
	case compressionTypeDXT5:
		return decodeDXT5DDS(h, r)
	default:
		return nil, fmt.Errorf("unsupported compression format %x", h.pixelFormat.fourCC)
	}

}

func decodeDXT5DDS(h header, r io.Reader) (image.Image, error) {
	imgBytes, err := io.ReadAll(r)
	if err != nil {
		return nil, err
	}

	rgbaPixels, err := decompressDxt5(imgBytes, int(h.width), int(h.height))
	if err != nil {
		return nil, err
	}

	rgbaBytes := make([]byte, len(rgbaPixels)*4)
	for i, pixel := range rgbaPixels {
		bi := i * 4
		rgbaBytes[bi] = pixel.R
		rgbaBytes[bi+1] = pixel.G
		rgbaBytes[bi+2] = pixel.B
		rgbaBytes[bi+3] = pixel.A
	}

	return &image.RGBA{
		Pix:    rgbaBytes,
		Stride: int(4 * h.width),
		Rect:   image.Rect(0, 0, int(h.width), int(h.height)),
	}, err
}

func decodeUncompressedDDS(h header, r io.Reader) (image.Image, error) {
	if h.pixelFormat.flags&(pfAlphaPixels|pfRGB) != pfAlphaPixels|pfRGB {
		return nil, fmt.Errorf("unsupported pixel format %x", h.pixelFormat.flags)
	}

	pitch := (h.width*h.pixelFormat.rgbBitCount + 7) / 8
	buf := make([]byte, pitch*h.height)
	if _, err := io.ReadFull(r, buf); err != nil {
		return nil, fmt.Errorf("reading image: %v", err)
	}
	stride := h.pixelFormat.rgbBitCount / 8

	return &img{
		h:   h,
		buf: buf,

		pitch:  int(pitch),
		stride: int(stride),

		rBit: lowestSetBit(h.pixelFormat.rBitMask),
		gBit: lowestSetBit(h.pixelFormat.gBitMask),
		bBit: lowestSetBit(h.pixelFormat.bBitMask),
		aBit: lowestSetBit(h.pixelFormat.aBitMask),
	}, nil
}

// This is a lazy hack to get dxt1 to work with this library
func decodeDXT1DDS(h header, r io.Reader) (image.Image, error) {
	imgBytes, err := io.ReadAll(r)
	if err != nil {
		return nil, err
	}

	rgbaBytes, err := dxt.DecodeDXT1(imgBytes, uint(h.width), uint(h.height))
	if err != nil {
		return nil, err
	}

	return &image.RGBA{
		Pix:    rgbaBytes,
		Stride: int(4 * h.width),
		Rect:   image.Rect(0, 0, int(h.width), int(h.height)),
	}, err
}
