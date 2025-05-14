package pdfocr

import (
	"bytes"
	"fmt"
	"io"
	"os"
	"strings"
)

// normalizeCoords rescales hOCR Bounding Box (bbox) coords to the PDF coords.
func normalizeCoords(x, y, hocrW, hocrH, pdfW, pdfH float64) (float64, float64) {
	nx := (x / hocrW) * pdfW
	ny := (y / hocrH) * pdfH
	return nx, ny
}

func unescapePDFString(s string) string {
	s = strings.ReplaceAll(s, "\\(", "(")
	s = strings.ReplaceAll(s, "\\)", ")")
	s = strings.ReplaceAll(s, "\\\\", "\\")
	return s
}

func decodeUTF16BE(b []byte) (string, error) {
	if len(b) < 2 {
		return "", fmt.Errorf("input too short for UTF-16BE")
	}
	hasBOM := false
	if b[0] == 0xFE && b[1] == 0xFF {
		hasBOM = true
		b = b[2:]
	}
	if !hasBOM {
		return "", fmt.Errorf("no BOM detected, cannot confirm UTF-16BE")
	}
	var runes []rune
	for i := 0; i < len(b); i += 2 {
		if i+1 >= len(b) {
			break
		}
		charCode := uint16(b[i])<<8 | uint16(b[i+1])
		runes = append(runes, rune(charCode))
	}
	return string(runes), nil
}

// getLogger returns the appropriate io.Writer to use for logging
// based on the configuration settings, defaulting to os.Stdout if nil.
func getLogger(config OCRConfig) io.Writer {
	if config.Logger == nil {
		return os.Stdout
	}
	return config.Logger
}

// dumpPDFStructure is a debug utility that prints out
// the first N bytes of the PDF plus any /OCG layer references.
func dumpPDFStructure(pdfData []byte, byteCount int, logger io.Writer) {
	if byteCount > len(pdfData) {
		byteCount = len(pdfData)
	}

	fmt.Fprintln(logger, "===== PDF STRUCTURE DUMP (FIRST", byteCount, "BYTES) =====")
	fmt.Fprintln(logger, string(pdfData[:byteCount]))
	fmt.Fprintln(logger, "===== END PDF STRUCTURE DUMP =====")

	ocgIndex := bytes.Index(pdfData, []byte("/OCG"))
	if ocgIndex >= 0 {
		start := ocgIndex - 20
		if start < 0 {
			start = 0
		}
		end := ocgIndex + 100
		if end > len(pdfData) {
			end = len(pdfData)
		}
		fmt.Fprintln(logger, "===== OCG CONTEXT =====")
		fmt.Fprintln(logger, string(pdfData[start:end]))
		fmt.Fprintln(logger, "===== END OCG CONTEXT =====")
	}
}
