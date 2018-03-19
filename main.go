package main

import (
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
	"github.com/malashin/gopsd/types"
	"golang.org/x/crypto/ssh/terminal"
)

var rgba string

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
		ansi.Println(fmt.Sprintf("\n\x1b[43;1m%03d\x1b[0m", i+1), args[i])

		// Open psd document.
		doc, err := gopsd.ParseFromPath(args[i])
		if err != nil {
			ansi.Println("\x1b[31;1m"+"    [gopsd.ParseFromPath]:", err, "\x1b[0m")
			hasErrors = true
			continue
		}

		// Skip file if color mode is not RGB.
		if doc.ColorMode != "RGB" {
			ansi.Println("\x1b[31;1m"+"    Documents ColorMode is "+doc.ColorMode+", must be RGB instead", "\x1b[0m")
			hasErrors = true
			continue
		}

		// Skip file if resolution is wrong.
		if (doc.Width != 1170 && doc.Width != 570) || doc.Height != 363 {
			ansi.Println("\x1b[31;1m" + "    Resolution must be 1170x363 or 570x363 (" + strconv.Itoa(int(doc.Width)) + "x" + strconv.Itoa(int(doc.Height)) + ")\x1b[0m")
			hasErrors = true
			continue
		}

		// Update all layers parents and children in the PSD document.
		doc.GetTreeRepresentation()

		// Remove all hidden layers from layers array.
		layers := doc.Layers
		layers, rgba, err = parseLayers(layers, int(doc.Width), int(doc.Height), args[i])
		if err != nil {
			ansi.Println("\x1b[31;1m"+"    [parseLayers]:", err, "\x1b[0m")
			hasErrors = true
			continue
		}

		// Report if \"#title_overlay_bg\" #RRGGBBAA values were not extracted.
		if rgba == "" {
			ansi.Println("\x1b[33;1m" + "    \"#title_overlay_bg\" layer not found" + "\x1b[0m")
		}

		// Skip file if there are no suitable layers to extract.
		if len(layers) == 0 {
			ansi.Println("\x1b[31;1m" + "    No suitable layers to extract." + "\x1b[0m")
			hasErrors = true
			continue
		}

		var pngs []string
		il := 1

		// If there is only one layer, don't number it and save as JPEG.
		if len(layers) == 1 {
			fileName, err := saveAsPNG(layers[0], int(doc.Width), int(doc.Height), args[i], 0)
			if err != nil {
				ansi.Println("\x1b[31;1m"+"    [saveAsPNG]:", err, "\x1b[0m")
				hasErrors = true
			}

			// Save PNG file as JPEG.
			saveAsJPEG(fileName)

			// Delete PNG file.
			err = os.Remove(fileName)
			if err != nil {
				ansi.Println("\x1b[31;1m"+"    [os.Remove]:", err, "\x1b[0m")
				hasErrors = true
			}
			continue
		}

		// Save layers to individual PNG files.
		for _, layer := range layers {
			fileName, err := saveAsPNG(layer, int(doc.Width), int(doc.Height), args[i], il)
			if err != nil {
				ansi.Println("\x1b[31;1m"+"    [saveAsPNG]:", err, "\x1b[0m")
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
			ansi.Println("\x1b[31;1m"+"    [savePreview]:", err, "\x1b[0m")
			hasErrors = true
			continue
		}
	}

	// If there were errors, don't close console window.
	if hasErrors {
		err := waitForAnyKey()
		ansi.Println("\x1b[31;1m"+"    [waitForAnyKey]:", err, "\x1b[0m")
	}
}

// waitForAnyKey await for any key press to continue.
func waitForAnyKey() error {
	fd := int(os.Stdin.Fd())
	if !terminal.IsTerminal(fd) {
		return fmt.Errorf("it's not a terminal descriptor")
	}
	state, err := terminal.MakeRaw(fd)
	if err != nil {
		return fmt.Errorf("cannot set raw mode")
	}
	defer terminal.Restore(fd, state)

	b := [1]byte{}
	os.Stdin.Read(b[:])
	return nil
}

// Check if layer or folders that contain that layer are visible.
func isLayerVisible(layer *gopsd.Layer) bool {
	if layer.IsSectionDivider || layer.ID == -1 {
		return false
	}

	if !layer.Visible || layer.Opacity == 0 {
		return false
	}

	if layer.Parent != nil && layer.Parent.ID != -1 {
		return isLayerVisible(layer.Parent)
	}

	return true
}

// parseLayers returns array of layers that are visible.
func parseLayers(layers []*gopsd.Layer, w, h int, filePath string) ([]*gopsd.Layer, string, error) {
	var dstLayers []*gopsd.Layer
	rgba := ""
	var err error

	for _, layer := range layers {
		// If overlay backround is found, store its color and opacity information as #RRGGBBAA.
		if layer.SolidColor != nil && layer.Name == "#title_overlay_bg" {
			rgba, err = solidColorToRRGGBBAA(layer)
			if err != nil {
				return []*gopsd.Layer{}, "", err
			}
		}

		// Skip hidden, invisible or empty layers.
		if !isLayerVisible(layer) {
			continue
		}

		// Return error if layer is a text layer.
		if layer.IsText() {
			return []*gopsd.Layer{}, "", fmt.Errorf("\"%v\" is a text layer", layer.Name)
		}

		// Get image from received layer.
		img, err := layer.GetImage()
		if err != nil {
			return []*gopsd.Layer{}, "", err
		}

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

	return dstLayers, rgba, nil
}

// saveAsPNG saves the layer as separate PNG.
func saveAsPNG(layer *gopsd.Layer, w, h int, filePath string, i int) (string, error) {
	// Skip hidden, invisible or empty layers.
	if !layer.Visible || layer.Opacity == 0 || layer.Rectangle.Width == 0 || layer.Rectangle.Height == 0 {
		return "", nil
	}

	// Get image from received layer.
	img, err := layer.GetImage()
	if err != nil {
		return "", err
	}

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
		if rgba != "" {
			fileName = filePath[0:len(filePath)-len(filepath.Ext(filePath))] + rgba + ".png"
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

		return fileName, nil
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

	ansi.Println("\x1b[32;1m" + "    " + filepath.Base(fileName) + "\x1b[0m")

	// // If this is the first layer (background without alpha channel) save it as JPEG.
	// if i == 1 {
	// 	err = saveAsJPEG(fileName)
	// 	if err != nil {
	// 		return "", err
	// 	}

	// 	// Delete PNG file.
	// 	err = os.Remove(fileName)
	// 	if err != nil {
	// 		return "", err
	// 	}
	// 	return fileName[0:len(fileName)-len(filepath.Ext(fileName))] + ".jpg", nil
	// }

	return fileName, nil
}

// savePreview overlays PNGs over each other and saves new image as PNG.
func savePreview(pngs []string, w, h int, filePath string) error {
	// Skip if PNG array is empty.
	if !(len(pngs) > 0) {
		return nil
	}

	// Create empty destination image.
	dst := image.NewNRGBA(image.Rect(0, 0, w, h))

	// Iterate over PNGs in array.
	for _, filePath := range pngs {
		// Open PNG file.
		file, err := os.Open(filePath)
		if err != nil {
			return err
		}

		// Decode it image.
		img, _, err := image.Decode(file)
		if err != nil {
			return err
		}

		// Overlay if over destination image.
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
	fileName := filePath[0:len(filePath)-len(filepath.Ext(filePath))] + ".png"
	if rgba != "" {
		fileName = filePath[0:len(filePath)-len(filepath.Ext(filePath))] + rgba + ".png"
	}
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

	// // Reduce the file size with lossy compression.
	// err = pngQuant(fileName)
	// if err != nil {
	// 	return err
	// }

	// Save preview file as JPEG.
	err = saveAsJPEG(fileName)
	if err != nil {
		return err
	}

	// Delete PNG preview file.
	err = os.Remove(fileName)
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

// saveAsJPEG uses ffmpeg to encode file to JPEG.
func saveAsJPEG(filePath string) error {
	// Get input file basename.
	basename := filePath[0 : len(filePath)-len(filepath.Ext(filePath))]

	// Run ffmpeg to encode file to JPEG.
	stdoutStderr, err := exec.Command("ffmpeg",
		"-i", filePath,
		"-q:v", "0",
		"-pix_fmt", "rgb24",
		"-map_metadata", "-1",
		"-loglevel", "error",
		"-y",
		basename+".jpg",
	).CombinedOutput()
	if err != nil {
		return err
	}
	if len(stdoutStderr) > 0 {
		return fmt.Errorf("%v", stdoutStderr)
	}

	ansi.Println("\x1b[32;1m" + "    " + filepath.Base(basename) + ".jpg" + "\x1b[0m")

	return nil
}

//solidColorToRRGGBBAA returns "#RRGGBBAA" string thats contains solid color layers RGB values and its opacity.
func solidColorToRRGGBBAA(layer *gopsd.Layer) (string, error) {
	rgba := ""
	if layer.SolidColor == nil {
		return rgba, fmt.Errorf("%q is not solid color", layer.Name)
	}

	clr, ok := layer.SolidColor.Items["Clr "]
	if !ok {
		return rgba, fmt.Errorf("%q has no \"Clr \" key", layer.Name)
	}

	descr, ok := clr.Value.(*types.Descriptor)
	if !ok {
		return rgba, fmt.Errorf("%q has no descriptor", layer.Name)
	}

	rd, ok := descr.Items["Rd  "]
	isOk := ok
	gr, ok := descr.Items["Grn "]
	isOk = isOk && ok
	bl, ok := descr.Items["Bl  "]
	isOk = isOk && ok
	if !isOk {
		return rgba, fmt.Errorf("%q color is not RGB", layer.Name)
	}

	r, ok := rd.Value.(float64)
	isOk = ok
	g, ok := gr.Value.(float64)
	isOk = isOk && ok
	b, ok := bl.Value.(float64)
	isOk = isOk && ok
	if !isOk {
		return rgba, fmt.Errorf("%q RGB values are not float64", layer.Name)
	}

	red := round(r)
	green := round(g)
	blue := round(b)
	if red < 0 || red > 255 ||
		green < 0 || green > 255 ||
		blue < 0 || blue > 255 {
		return rgba, fmt.Errorf("%q RGB values are not in 0-255 range: <%v,%v,%v>", layer.Name, red, green, blue)
	}
	alpha := round(float64(layer.Opacity) / 100 * 255)
	rgba = fmt.Sprintf("#%02x%02x%02x%02x", red, green, blue, alpha)
	return rgba, nil
}

// round rounds floats into integer numbers.
func round(input float64) int {
	if input < 0 {
		return int(math.Ceil(input - 0.5))
	}
	return int(math.Floor(input + 0.5))
}

// help returns usage information.
func help() {
	ansi.Println("rtpng saves each visible rasterized PSD layer as separate PNG file and combines them into one preview JPEG file.")
	ansi.Println("usage: rtpng file1.psd [file2.psd]...")
}
