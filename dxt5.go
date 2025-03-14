package dds

import (
	"bytes"
	"encoding/binary"
	"github.com/hugespaceship/dds/common"
	"image/color"
)

const blockSize = 4

// decompressDxt5
// Decompress a Dxt5 compressed slice of bytes.
// Decompresses block by block
// Width and Height are required, as this information is impossible to derive with
// 100% accuracy (e.g. 256x1024 cannot be distinguished from 512x512) from raw alone
func decompressDxt5(packed []byte, width int, height int) ([]color.RGBA, error) {
	unpacked := make([]color.RGBA, width*height)

	blockCountX := int((width + 3) / blockSize)
	blockCountY := int((height + 3) / blockSize)

	offset := 0
	for j := 0; j < blockCountY; j++ {
		for i := 0; i < blockCountX; i++ {
			if err := decompressDxt5Block(packed[offset+(i*16):], i*blockSize, j*blockSize, width, unpacked); err != nil {
				return nil, err
			}
		}
		offset += blockCountX * 16
	}

	return unpacked, nil
}

// decompressDxt5Block
// decompress a single dxt5 compressed block.
// A single decompressed block is 4x4 pixels located at x,y location in the resultant image
func decompressDxt5Block(packed []byte, offsetX int, offsetY int, width int, unpacked []color.RGBA) error {
	var alpha0, alpha1 uint8
	err := binary.Read(bytes.NewBuffer(packed[:1]), binary.LittleEndian, &alpha0)
	if err != nil {
		return err
	}
	err = binary.Read(bytes.NewBuffer(packed[1:2]), binary.LittleEndian, &alpha1)
	if err != nil {
		return err
	}

	var bits [6]uint8
	err = binary.Read(bytes.NewBuffer(packed[2:8]), binary.LittleEndian, &bits)
	if err != nil {
		return err
	}

	alphaCode1 := uint32(bits[2]) | (uint32(bits[3]) << 8) | (uint32(bits[4]) << 16) | (uint32(bits[5]) << 24)
	alphaCode2 := uint16(bits[0]) | (uint16(bits[1]) << 8)

	// Construct colours to transform between
	var c0, c1 uint16
	err = binary.Read(bytes.NewBuffer(packed[8:10]), binary.LittleEndian, &c0)
	if err != nil {
		return err
	}
	err = binary.Read(bytes.NewBuffer(packed[10:12]), binary.LittleEndian, &c1)
	if err != nil {
		return err
	}

	colour0 := common.Rgb565toargb8888(c0)
	colour1 := common.Rgb565toargb8888(c1)

	var code uint32
	err = binary.Read(bytes.NewBuffer(packed[12:16]), binary.LittleEndian, &code)
	if err != nil {
		return err
	}

	for j := 0; j < blockSize; j++ {
		for i := 0; i < blockSize; i++ {
			alphaCodeIndex := uint(3 * (4*j + i))
			var alphaCode int

			if alphaCodeIndex <= 12 {
				alphaCode = int((alphaCode2 >> alphaCodeIndex) & 0x07)
			} else if alphaCodeIndex == 15 {
				alphaCode = int((uint32(alphaCode2) >> 15) | ((alphaCode1 << 1) & 0x06))
			} else {
				// alphaCodeIndex >= 18 && alphaCodeIndex <= 45
				alphaCode = int((alphaCode1 >> (alphaCodeIndex - 16)) & 0x07)
			}

			var finalAlpha uint8
			if alphaCode == 0 {
				finalAlpha = alpha0
			} else if alphaCode == 1 {
				finalAlpha = alpha1
			} else {
				if alpha0 > alpha1 {
					finalAlpha = ((8-uint8(alphaCode))*alpha0 + (uint8(alphaCode)-1)*alpha1) / 7
				} else {
					if alphaCode == 6 {
						finalAlpha = 0
					} else if alphaCode == 7 {
						finalAlpha = 255
					} else {
						finalAlpha = ((6-uint8(alphaCode))*alpha0 + (uint8(alphaCode)-1)*alpha1) / 5
					}
				}
			}

			colorCode := (code >> uint32(2*(4*j+i))) & 0x03

			var finalColour color.RGBA
			switch colorCode {
			case 0:
				finalColour = colour0
			case 1:
				finalColour = colour1
			case 2:
				finalColour = color.RGBA{
					R: (2*colour0.R + colour1.R) / 3,
					G: (2*colour0.G + colour1.G) / 3,
					B: (2*colour0.B + colour1.B) / 3,
				}
			case 3:
				finalColour = color.RGBA{
					R: (colour0.R + 2*colour1.R) / 3,
					G: (colour0.G + 2*colour1.G) / 3,
					B: (colour0.B + 2*colour1.B) / 3,
				}
			}

			if finalAlpha != 255 {
				a := 0
				a -= 2
			}

			// Set alpha
			finalColour.A = finalAlpha

			if offsetX+i < width {
				unpacked[(offsetY+j)*width+(offsetX+i)] = finalColour
			}
		}
	}

	return nil
}
