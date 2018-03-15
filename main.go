package main

import (
	"bufio"
	"fmt"
	"image"
	"image/draw"
	"image/png"
	"math"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"

	ansi "github.com/k0kubun/go-ansi"
	"github.com/malashin/gopsd"
)

func main() {
	// Convert passed arguments into array.
	args := os.Args[1:]

	// If program is executed without arguments.
	if len(args) < 1 {
		// Show usage information.
		help()
		os.Exit(0)
	}

	hasErrors := false

	// Iterate over passed arguments.
	for i := 0; i < len(args); i++ {
		// Print out file name.
		ansi.Println(fmt.Sprintf("%03d", i+1), args[i])

		// Open psd document.
		doc, err := gopsd.ParseFromPath(args[i])
		if err != nil {
			ansi.Println("\x1b[31;1m"+args[i]+" [gopsd.ParseFromPath]:", err, "\x1b[0m")
			hasErrors = true
			continue
		}

		// Skip file if resolution is wrong.
		if (doc.Width != 1170 && doc.Width != 570) || doc.Height != 363 {
			ansi.Println("\x1b[31;1m" + args[i] + ": resolution must be 1170x363 or 570x363 (" + strconv.Itoa(int(doc.Width)) + "x" + strconv.Itoa(int(doc.Height)) + ")\x1b[0m")
			hasErrors = true
			continue
		}

		// Remove all hidden layers from layers array.
		layers := doc.Layers
		layers, err = removeHiddenLayers(layers, int(doc.Width), int(doc.Height), args[i])
		if err != nil {
			ansi.Println("\x1b[31;1m"+args[i]+" [removeHiddenLayers]:", err, "\x1b[0m")
			hasErrors = true
			continue
		}

		var pngs []string
		il := 1

		// If there is only one layer, don't number it.
		if len(layers) == 1 {
			_, err := saveAsPNG(layers[0], int(doc.Width), int(doc.Height), args[i], 0)
			if err != nil {
				ansi.Println("\x1b[31;1m"+args[i]+" [saveAsPNG]:", err, "\x1b[0m")
				hasErrors = true
			}
			continue
		}

		// Save layers to individual PNG files.
		for _, layer := range layers {
			fileName, err := saveAsPNG(layer, int(doc.Width), int(doc.Height), args[i], il)
			if err != nil {
				ansi.Println("\x1b[31;1m"+args[i]+" [saveAsPNG]:", err, "\x1b[0m")
				hasErrors = true
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
			ansi.Println("\x1b[31;1m"+args[i]+" [savePreview]:", err, "\x1b[0m")
			hasErrors = true
			continue
		}
	}

	// If there were errors, don't close console window.
	if hasErrors {
		ansi.Println("\nPress 'Enter' to continue...")
		bufio.NewReader(os.Stdin).ReadBytes('\n')
	}
}

// removeHiddenLayers returns array of layers that are visible.
func removeHiddenLayers(layers []*gopsd.Layer, w, h int, filePath string) ([]*gopsd.Layer, error) {
	var dstLayers []*gopsd.Layer

	for _, layer := range layers {
		// Skip hidden, invisable or empty layers.
		if !layer.Visible || layer.Opacity == 0 || layer.Rectangle.Width == 0 || layer.Rectangle.Height == 0 {
			continue
		}

		// Return error if layer was not rastersized beforehand.
		if layer.Type == gopsd.TypeShape {
			return []*gopsd.Layer{}, fmt.Errorf("\"%v\" has wrong layerType", layer.Name)
		}

		// Get image from recived layer.
		img, err := layer.GetImage()
		if err != nil {
			return []*gopsd.Layer{}, err
		}

		// // Premultiply recived layer.
		// rgba := img.(*image.RGBA)
		// for pi := 0; pi < len(rgba.Pix); pi += 4 {
		// 	a := float64(rgba.Pix[pi+3]) / 255
		// 	rgba.Pix[pi] = uint8(round(float64(rgba.Pix[pi]) * a))
		// 	rgba.Pix[pi+1] = uint8(round(float64(rgba.Pix[pi+1]) * a))
		// 	rgba.Pix[pi+2] = uint8(round(float64(rgba.Pix[pi+2]) * a))
		// }

		// Create empty image.
		dst := image.NewNRGBA(image.Rect(0, 0, w, h))

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
		if dst.Opaque() && len(dstLayers) > 0 {
			ansi.Println("\x1b[31;1m" + filePath + ": layers behind \"" + layer.Name + "\" are hidden" + "\x1b[0m")
			dstLayers = []*gopsd.Layer{layer}
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

	// Get image from recived layer.
	img, err := layer.GetImage()
	if err != nil {
		return "", err
	}

	// // Premultiply recived layer.
	// rgba := img.(*image.RGBA)
	// for pi := 0; pi < len(rgba.Pix); pi += 4 {
	// 	a := float64(rgba.Pix[pi+3]) / 255
	// 	rgba.Pix[pi] = uint8(round(float64(rgba.Pix[pi]) * a))
	// 	rgba.Pix[pi+1] = uint8(round(float64(rgba.Pix[pi+1]) * a))
	// 	rgba.Pix[pi+2] = uint8(round(float64(rgba.Pix[pi+2]) * a))
	// }

	// Create empty image.
	dst := image.NewNRGBA(image.Rect(0, 0, w, h))

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
	// Don't number the file if there is only one layer.
	if i == 0 {
		fileName = filePath[0:len(filePath)-len(filepath.Ext(filePath))] + ".png"
	}
	out, err := os.Create(fileName)
	if err != nil {
		return "", err
	}

	// Save image as PNG to this file.
	err = png.Encode(out, dst)
	if err != nil {
		return "", err
	}
	out.Close()

	// Reduce the file size with lossy compression.
	err = pngQuant(fileName)
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
	dst := image.NewNRGBA(image.Rect(0, 0, w, h))

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
	fileName := filePath[0:len(filePath)-len(filepath.Ext(filePath))] + "_MERGED.png"
	out, err := os.Create(fileName)
	if err != nil {
		return err
	}

	// Save image as PNG to this file.
	err = png.Encode(out, dst)
	if err != nil {
		return err
	}
	out.Close()

	// Reduce the file size with lossy compression.
	err = pngQuant(fileName)
	if err != nil {
		return err
	}

	return nil
}

// pngQuant reduces the file size of input PNG file with lossy compression.
func pngQuant(filePath string) error {
	// Get input file basename.
	basename := filePath[0 : len(filePath)-len(filepath.Ext(filePath))]

	// Run pngquant to reduce the file size of input PNG file with lossy compression.
	stdoutStderr, err := exec.Command("pngquant",
		"--force",
		"--skip-if-larger",
		"--output", basename+"####.png",
		"--quality=0-100",
		"--speed", "1",
		"--strip",
		"--", filePath,
	).CombinedOutput()
	if err != nil {
		return err
	}
	if len(stdoutStderr) > 0 {
		return fmt.Errorf("%v", stdoutStderr)
	}

	// Rename new PNG to match input files name.
	err = os.Rename(basename+"####.png", filePath)
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

// help returns usage information.
func help() {
	ansi.Println("rtpng saves each visible rasterized PSD layer as separate PNG file and combines them into one preview PNG file.")
	ansi.Println("usage: rtpng file1.psd [file2.psd]...")
}
