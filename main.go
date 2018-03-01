package main

import (
	"fmt"
	"image"
	"image/draw"
	"image/png"
	"math"
	"os"
	"path/filepath"
	"strconv"

	ansi "github.com/k0kubun/go-ansi"
	"github.com/solovev/gopsd"
)

func main() {
	// Convert passed arguments into array.
	args := os.Args[1:]

	// Iterate over passed arguments.
	for i := 0; i < len(args); i++ {
		doc, err := gopsd.ParseFromPath(args[i])
		if err != nil {
			ansi.Println("\x1b[31;1m"+args[i]+":", err, "\x1b[0m")
			continue
		}

		// Skip file if resolution is wrong.
		if (doc.Width != 1170 && doc.Width != 570) || doc.Height != 363 {
			ansi.Println("\x1b[31;1m" + args[i] + ": resolution must be 1170x363 or 570x363 (" + strconv.Itoa(int(doc.Width)) + "x" + strconv.Itoa(int(doc.Height)) + ")\x1b[0m")
			continue
		}

		// Remove all hidden layers from layers array.
		layers := doc.Layers
		layers, err = removeHiddenLayers(layers, int(doc.Width), int(doc.Height), args[i])
		if err != nil {
			ansi.Println("\x1b[31;1m"+args[i]+":", err, "\x1b[0m")
			continue
		}

		var pngs []string
		il := 1

		// Save layers to individual PNG files.
		for _, layer := range layers {
			fileName, err := saveAsPNG(layer, int(doc.Width), int(doc.Height), args[i], il)
			if err != nil {
				ansi.Println("\x1b[31;1m"+args[i]+":", err, "\x1b[0m")
				continue
			}

			// Store path to saved PNGs.
			if fileName != "" {
				pngs = append(pngs, fileName)
				il++
			}
		}

		// Create preview image with overlayed PNGs.
		err = savePreview(pngs, int(doc.Width), int(doc.Height), args[i])
		if err != nil {
			ansi.Println("\x1b[31;1m"+args[i]+":", err, "\x1b[0m")
			continue
		}
	}
}

// removeHiddenLayers returns array of layers that are visible.
func removeHiddenLayers(layers []*gopsd.Layer, w, h int, filePath string) ([]*gopsd.Layer, error) {
	var dstLayers []*gopsd.Layer

	for i, layer := range layers {
		// Skip hidden, invisable or empty layers.
		if !layer.Visible || layer.Opacity == 0 || layer.Rectangle.Width == 0 || layer.Rectangle.Height == 0 {
			continue
		}

		// Get image from recived layer.
		img, err := layer.GetImage()
		if err != nil {
			return []*gopsd.Layer{}, err
		}

		// Premultiply recived layer.
		rgba := img.(*image.RGBA)
		for pi := 0; pi < len(rgba.Pix); pi += 4 {
			a := float64(rgba.Pix[pi+3]) / 255
			rgba.Pix[pi] = uint8(round(float64(rgba.Pix[pi]) * a))
			rgba.Pix[pi+1] = uint8(round(float64(rgba.Pix[pi+1]) * a))
			rgba.Pix[pi+2] = uint8(round(float64(rgba.Pix[pi+2]) * a))
		}

		// Create empty image.
		dst := image.NewRGBA(image.Rect(0, 0, w, h))

		// Draw premultiplied layer over empty image.
		draw.Draw(
			dst,
			image.Rectangle{
				Min: image.Point{int(layer.Rectangle.X), int(layer.Rectangle.Y)},
				Max: dst.Rect.Max},
			img,
			image.ZP,
			draw.Src,
		)

		// If layer is opaque and there were layers behind it, remove those hidden layers.
		if dst.Opaque() && i != 0 {
			ansi.Println("\x1b[31;1m" + filePath + ": layers behind \"" + layer.Name + "\" are hidden" + "\x1b[0m")
			dstLayers = append([]*gopsd.Layer{}, layer)
		} else {
			dstLayers = append(dstLayers, layer)
		}
	}

	return dstLayers, nil
}

// saveAsPNG saves the layer as separate PNG.
func saveAsPNG(layer *gopsd.Layer, w, h int, filePath string, i int) (string, error) {
	// Skip hidden, invisable or empty layers.
	if !layer.Visible || layer.Opacity == 0 || layer.Rectangle.Width == 0 || layer.Rectangle.Height == 0 {
		return "", nil
	}

	// Return error if layer was not rastersized beforehand.
	if layer.Type != gopsd.TypeUnspecified && layer.Type != gopsd.TypeDefault {
		return "", fmt.Errorf("\"%v\" has wrong layerType", layer.Name)
	}

	// Get image from recived layer.
	img, err := layer.GetImage()
	if err != nil {
		return "", err
	}

	// Premultiply recived layer.
	rgba := img.(*image.RGBA)
	for pi := 0; pi < len(rgba.Pix); pi += 4 {
		a := float64(rgba.Pix[pi+3]) / 255
		rgba.Pix[pi] = uint8(round(float64(rgba.Pix[pi]) * a))
		rgba.Pix[pi+1] = uint8(round(float64(rgba.Pix[pi+1]) * a))
		rgba.Pix[pi+2] = uint8(round(float64(rgba.Pix[pi+2]) * a))
	}

	// Create empty image.
	dst := image.NewRGBA(image.Rect(0, 0, w, h))

	// Draw premultiplied layer over empty image.
	draw.Draw(
		dst,
		image.Rectangle{
			Min: image.Point{int(layer.Rectangle.X), int(layer.Rectangle.Y)},
			Max: dst.Rect.Max},
		img,
		image.ZP,
		draw.Src,
	)

	// Create new file.
	fileName := filePath[0:len(filePath)-len(filepath.Ext(filePath))] + "_" + fmt.Sprintf("%02d", i) + ".png"
	out, err := os.Create(fileName)
	if err != nil {
		return "", err
	}

	// Save image as PNG to this file.
	err = png.Encode(out, dst)
	if err != nil {
		return "", err
	}
	return fileName, nil
}

// savePreview overlays PNGs over each other and saves new image as PNG.
func savePreview(pngs []string, w, h int, filePath string) error {
	// Skip if PNG array is empty.
	if !(len(pngs) > 0) {
		return nil
	}

	// Create empty distanation image.
	dst := image.NewRGBA(image.Rect(0, 0, w, h))

	// Iterate over PNGs in array.
	for _, filePath := range pngs {
		// Open PNG file.
		file, err := os.Open(filePath)
		if err != nil {
			return err
		}

		// Decode it as PNG.
		img, err := png.Decode(file)
		if err != nil {
			return err
		}

		// Overlay if over distanation image.
		draw.Draw(
			dst,
			dst.Bounds(),
			img,
			image.ZP,
			draw.Over,
		)

		// Close the PNG file.
		file.Close()
	}

	// Create new file.
	fileName := filePath[0:len(filePath)-len(filepath.Ext(filePath))] + "_PREVIEW.png"
	out, err := os.Create(fileName)
	if err != nil {
		return err
	}

	// Save image as PNG to this file.
	err = png.Encode(out, dst)
	if err != nil {
		return err
	}
	return nil
}

// round rounds floats into integer numbers.
func round(input float64) int64 {
	if input < 0 {
		return int64(math.Ceil(input - 0.5))
	}
	return int64(math.Floor(input + 0.5))
}
