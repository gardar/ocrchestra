// Package hocr implements parsing, manipulation, and generation of hOCR data,
// which is an HTML-based standard format for representing OCR results.
//
// This package provides:
//
// - A complete object model representing the hOCR hierarchy
// - Functions for parsing hOCR HTML into structured Go types
// - Functions for generating valid hOCR HTML from Go structures
// - Utilities for working with bounding boxes and positional data
//
// The package implements the hierarchical structure defined in the hOCR format:
// Document → Pages → Areas → Paragraphs → Lines → Words, with metadata at each level.
//
// Key Types:
//
// - HOCR: Top-level structure representing an entire hOCR document
// - Page: Represents a single page with class 'ocr_page'
// - Area: Represents a content area with class 'ocr_carea'
// - Paragraph: Represents a paragraph with class 'ocr_par'
// - Line: Represents a line of text with class 'ocr_line'
// - Word: Represents a single word with class 'ocrx_word'
// - BoundingBox: Represents a rectangle with coordinates for positioning elements
//
// Main Functions:
//
// - ParseHOCR: Parses hOCR data from HTML into the object model
// - GenerateHOCRDocument: Generates valid hOCR HTML from the object model
package hocr
