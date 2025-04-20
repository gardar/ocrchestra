package hocr

import (
	"fmt"
	"strconv"
	"strings"

	"golang.org/x/net/html"
	"golang.org/x/text/encoding/charmap"
)

// ParseHOCR converts raw hOCR data into a structured HOCR object.
func ParseHOCR(data []byte) (HOCR, error) {
	var result HOCR
	result.Metadata = make(map[string]string)

	// Figure out the character encoding
	content := string(data)
	encoding := "utf-8"
	if strings.Contains(content, "charset=") {
		metaStart := strings.Index(content, "charset=") + len("charset=")
		if metaStart > -1 && len(content) > metaStart+10 {
			encSnippet := content[metaStart : metaStart+20]
			enc := strings.ToLower(strings.FieldsFunc(encSnippet, func(r rune) bool {
				return r == '"' || r == ';' || r == '\'' || r == '>'
			})[0])
			if enc != "" {
				encoding = enc
			}
		}
	}

	// Convert to UTF-8 if needed
	var decoded []byte
	var err error
	if encoding != "utf-8" {
		decoder := charmap.ISO8859_1.NewDecoder()
		decoded, err = decoder.Bytes(data)
		if err != nil {
			return result, fmt.Errorf("failed to decode %s: %w", encoding, err)
		}
	} else {
		decoded = data
	}

	doc, err := html.Parse(strings.NewReader(string(decoded)))
	if err != nil {
		return result, err
	}

	// Extract document metadata from the head section
	extractDocumentMeta(&result, doc)

	// Find and process all ocr_page elements
	var findPages func(*html.Node)
	findPages = func(n *html.Node) {
		if n.Type == html.ElementNode && n.Data == "div" {
			isOcrPage := false
			for _, a := range n.Attr {
				if a.Key == "class" && strings.Contains(a.Val, "ocr_page") {
					isOcrPage = true
					break
				}
			}
			if isOcrPage {
				page, err := processPage(n)
				if err == nil {
					result.Pages = append(result.Pages, page)
				}
				return
			}
		}
		for c := n.FirstChild; c != nil; c = c.NextSibling {
			findPages(c)
		}
	}
	findPages(doc)

	if len(result.Pages) == 0 {
		return result, fmt.Errorf("no ocr_page elements found in HOCR data")
	}
	return result, nil
}

// ParseTitle breaks down an hOCR title attribute into its components
// Example input: "bbox 100 200 300 400; x_wconf 95"
func ParseTitle(title string) map[string][]string {
	result := make(map[string][]string)
	parts := strings.Split(title, ";")
	for _, part := range parts {
		part = strings.TrimSpace(part)
		if part == "" {
			continue
		}

		items := strings.Fields(part)
		if len(items) > 0 {
			key := items[0]
			values := items[1:]
			result[key] = values
		}
	}

	return result
}

// ParseBoundingBoxFromTitle extracts a bounding box from a title string
// Returns a structured BoundingBox object or nil if extraction fails
func ParseBoundingBoxFromTitle(title string) *BoundingBox {
	props := ParseTitle(title)
	if bbox, ok := props["bbox"]; ok && len(bbox) >= 4 {
		x1, _ := strconv.ParseFloat(bbox[0], 64)
		y1, _ := strconv.ParseFloat(bbox[1], 64)
		x2, _ := strconv.ParseFloat(bbox[2], 64)
		y2, _ := strconv.ParseFloat(bbox[3], 64)
		result := NewBoundingBox(x1, y1, x2, y2)
		return &result
	}
	return nil
}

// extractDocumentMeta extracts document-level metadata from the head section
func extractDocumentMeta(result *HOCR, doc *html.Node) {
	var findHead func(*html.Node) *html.Node
	findHead = func(n *html.Node) *html.Node {
		if n.Type == html.ElementNode && n.Data == "head" {
			return n
		}
		for c := n.FirstChild; c != nil; c = c.NextSibling {
			if found := findHead(c); found != nil {
				return found
			}
		}
		return nil
	}

	// Check for lang attribute on the html tag
	var findHTMLLang func(*html.Node)
	findHTMLLang = func(n *html.Node) {
		if n.Type == html.ElementNode && n.Data == "html" {
			for _, a := range n.Attr {
				if a.Key == "lang" || a.Key == "xml:lang" {
					result.Language = a.Val
					return
				}
			}
		}
		// Only check direct children of the document node
		if n.Parent == nil {
			for c := n.FirstChild; c != nil; c = c.NextSibling {
				findHTMLLang(c)
			}
		}
	}
	findHTMLLang(doc)

	head := findHead(doc)
	if head == nil {
		return
	}

	// Extract title, language, description, etc.
	for c := head.FirstChild; c != nil; c = c.NextSibling {
		if c.Type == html.ElementNode {
			switch c.Data {
			case "title":
				if c.FirstChild != nil {
					result.Title = c.FirstChild.Data
				}
			case "meta":
				name := ""
				content := ""
				for _, attr := range c.Attr {
					if attr.Key == "name" {
						name = attr.Val
					} else if attr.Key == "content" {
						content = attr.Val
					}
				}
				if name != "" && content != "" {
					if name == "ocr-system" || name == "ocr-capabilities" ||
						name == "ocr-number-of-pages" || name == "ocr-langs" {
						result.Metadata[name] = content
					} else if name == "description" {
						result.Description = content
					} else if name == "dc.language" {
						result.Language = content
					}
				}
			}
		}
	}
}

// processPage extracts page information and its children (areas, lines, words)
func processPage(n *html.Node) (Page, error) {
	page := Page{
		Metadata: make(map[string]string),
	}

	// Extract page attributes
	for _, attr := range n.Attr {
		if attr.Key == "id" {
			page.ID = attr.Val
		} else if attr.Key == "lang" {
			page.Lang = attr.Val
		} else if attr.Key == "title" {
			page.Title = attr.Val

			// Extract bbox using the ParseBoundingBoxFromTitle function
			if bbox := ParseBoundingBoxFromTitle(attr.Val); bbox != nil {
				page.BBox = *bbox
			}

			// Extract other properties from title
			props := ParseTitle(attr.Val)
			if image, ok := props["image"]; ok && len(image) > 0 {
				page.ImageName = image[0]
			}
			if ppageno, ok := props["ppageno"]; ok && len(ppageno) > 0 {
				page.PageNumber, _ = strconv.Atoi(ppageno[0])
			}
		}
	}

	// Process areas, paragraphs, lines directly under the page
	var areaNodes []*html.Node
	var paragraphNodes []*html.Node
	var lineNodes []*html.Node

	var collectNodes func(*html.Node)
	collectNodes = func(node *html.Node) {
		if node.Type == html.ElementNode {
			class := getAttrVal(node, "class")
			if strings.Contains(class, "ocr_carea") {
				areaNodes = append(areaNodes, node)
				return
			} else if strings.Contains(class, "ocr_par") {
				paragraphNodes = append(paragraphNodes, node)
				return
			} else if strings.Contains(class, "ocr_line") {
				lineNodes = append(lineNodes, node)
				return
			}
		}
		for c := node.FirstChild; c != nil; c = c.NextSibling {
			collectNodes(c)
		}
	}

	for c := n.FirstChild; c != nil; c = c.NextSibling {
		collectNodes(c)
	}

	// Process areas
	for _, areaNode := range areaNodes {
		area, err := processArea(areaNode)
		if err == nil {
			page.Areas = append(page.Areas, area)
		}
	}

	// Process paragraphs directly under the page
	for _, paragraphNode := range paragraphNodes {
		paragraph, err := processParagraph(paragraphNode)
		if err == nil {
			page.Paragraphs = append(page.Paragraphs, paragraph)
		}
	}

	// Process any lines that don't belong to an area, block, or paragraph
	for _, lineNode := range lineNodes {
		line, err := processLine(lineNode)
		if err == nil {
			page.Lines = append(page.Lines, line)
		}
	}

	return page, nil
}

// processArea extracts area information and its children (paragraphs, lines, words)
func processArea(n *html.Node) (Area, error) {
	area := Area{
		Metadata: make(map[string]string),
	}

	// Extract area attributes
	for _, attr := range n.Attr {
		if attr.Key == "id" {
			area.ID = attr.Val
		} else if attr.Key == "lang" {
			area.Lang = attr.Val
		} else if attr.Key == "title" {
			// Extract bounding box using the ParseBoundingBoxFromTitle function
			if bbox := ParseBoundingBoxFromTitle(attr.Val); bbox != nil {
				area.BBox = *bbox
			}

			// Store other properties in metadata
			props := ParseTitle(attr.Val)
			for k, v := range props {
				if k != "bbox" {
					area.Metadata[k] = strings.Join(v, " ")
				}
			}
		}
	}

	// Find paragraphs, lines and words in this area
	var paragraphNodes []*html.Node
	var lineNodes []*html.Node
	var wordNodes []*html.Node

	var collectNodes func(*html.Node)
	collectNodes = func(node *html.Node) {
		if node.Type == html.ElementNode {
			class := getAttrVal(node, "class")
			if strings.Contains(class, "ocr_par") {
				paragraphNodes = append(paragraphNodes, node)
				return
			} else if strings.Contains(class, "ocr_line") {
				lineNodes = append(lineNodes, node)
				return
			} else if strings.Contains(class, "ocrx_word") {
				wordNodes = append(wordNodes, node)
				return
			}
		}
		for c := node.FirstChild; c != nil; c = c.NextSibling {
			collectNodes(c)
		}
	}

	for c := n.FirstChild; c != nil; c = c.NextSibling {
		collectNodes(c)
	}

	// Process paragraphs
	for _, paragraphNode := range paragraphNodes {
		paragraph, err := processParagraph(paragraphNode)
		if err == nil {
			area.Paragraphs = append(area.Paragraphs, paragraph)
		}
	}

	// Process lines that are directly under the area
	for _, lineNode := range lineNodes {
		line, err := processLine(lineNode)
		if err == nil {
			area.Lines = append(area.Lines, line)
		}
	}

	// Process any words directly under the area (no parent line)
	for _, wordNode := range wordNodes {
		word, err := processWord(wordNode)
		if err == nil {
			area.Words = append(area.Words, word)
		}
	}

	return area, nil
}

// processParagraph extracts paragraph information and its children (lines, words)
func processParagraph(n *html.Node) (Paragraph, error) {
	paragraph := Paragraph{
		Metadata: make(map[string]string),
	}

	// Extract paragraph attributes
	for _, attr := range n.Attr {
		if attr.Key == "id" {
			paragraph.ID = attr.Val
		} else if attr.Key == "lang" {
			paragraph.Lang = attr.Val
		} else if attr.Key == "title" {
			// Extract bounding box using the ParseBoundingBoxFromTitle function
			if bbox := ParseBoundingBoxFromTitle(attr.Val); bbox != nil {
				paragraph.BBox = *bbox
			}

			// Store other properties in metadata
			props := ParseTitle(attr.Val)
			for k, v := range props {
				if k != "bbox" {
					paragraph.Metadata[k] = strings.Join(v, " ")
				}
			}
		}
	}

	// Find lines and words in this paragraph
	var lineNodes []*html.Node
	var wordNodes []*html.Node

	var collectNodes func(*html.Node)
	collectNodes = func(node *html.Node) {
		if node.Type == html.ElementNode {
			class := getAttrVal(node, "class")
			if strings.Contains(class, "ocr_line") {
				lineNodes = append(lineNodes, node)
				return
			} else if strings.Contains(class, "ocrx_word") {
				wordNodes = append(wordNodes, node)
				return
			}
		}
		for c := node.FirstChild; c != nil; c = c.NextSibling {
			collectNodes(c)
		}
	}

	for c := n.FirstChild; c != nil; c = c.NextSibling {
		collectNodes(c)
	}

	// Process lines
	for _, lineNode := range lineNodes {
		line, err := processLine(lineNode)
		if err == nil {
			paragraph.Lines = append(paragraph.Lines, line)
		}
	}

	// Process any words directly under the paragraph (no parent line)
	for _, wordNode := range wordNodes {
		word, err := processWord(wordNode)
		if err == nil {
			paragraph.Words = append(paragraph.Words, word)
		}
	}

	return paragraph, nil
}

// processLine extracts line information and its words
func processLine(n *html.Node) (Line, error) {
	line := Line{
		Metadata: make(map[string]string),
	}

	// Extract line attributes
	for _, attr := range n.Attr {
		if attr.Key == "id" {
			line.ID = attr.Val
		} else if attr.Key == "lang" {
			line.Lang = attr.Val
		} else if attr.Key == "title" {
			// Extract bounding box using the ParseBoundingBoxFromTitle function
			if bbox := ParseBoundingBoxFromTitle(attr.Val); bbox != nil {
				line.BBox = *bbox
			}

			// Extract other properties from title
			props := ParseTitle(attr.Val)
			if baseline, ok := props["baseline"]; ok && len(baseline) > 0 {
				line.Baseline = strings.Join(baseline, " ")
			}

			// Store other properties in metadata
			for k, v := range props {
				if k != "bbox" && k != "baseline" {
					line.Metadata[k] = strings.Join(v, " ")
				}
			}
		}
	}

	// Process all word elements in this line
	var extractWords func(*html.Node)
	extractWords = func(node *html.Node) {
		if node.Type == html.ElementNode {
			for _, a := range node.Attr {
				if a.Key == "class" && strings.Contains(a.Val, "ocrx_word") {
					word, err := processWord(node)
					if err == nil {
						line.Words = append(line.Words, word)
					}
					return
				}
			}
		}
		for c := node.FirstChild; c != nil; c = c.NextSibling {
			extractWords(c)
		}
	}

	for c := n.FirstChild; c != nil; c = c.NextSibling {
		extractWords(c)
	}

	return line, nil
}

// Process a word element and extract its text and properties
func processWord(n *html.Node) (Word, error) {
	word := Word{
		Metadata: make(map[string]string),
	}

	// Extract word attributes
	for _, attr := range n.Attr {
		if attr.Key == "id" {
			word.ID = attr.Val
		} else if attr.Key == "lang" {
			word.Lang = attr.Val
		} else if attr.Key == "title" {
			// Extract bounding box using the ParseBoundingBoxFromTitle function
			if bbox := ParseBoundingBoxFromTitle(attr.Val); bbox != nil {
				word.BBox = *bbox
			}

			// Extract other properties from title
			props := ParseTitle(attr.Val)
			if conf, ok := props["x_wconf"]; ok && len(conf) > 0 {
				word.Confidence, _ = strconv.ParseFloat(conf[0], 64)
			}
			if lang, ok := props["lang"]; ok && len(lang) > 0 {
				word.Lang = lang[0]
			}

			// Store other properties in metadata
			for k, v := range props {
				if k != "bbox" && k != "x_wconf" && k != "lang" {
					word.Metadata[k] = strings.Join(v, " ")
				}
			}
		}
	}

	// Get the actual text content
	if n.FirstChild != nil {
		word.Text = extractTextContent(n)
	}

	return word, nil
}

// extractTextContent gets all text from a node and its children
func extractTextContent(n *html.Node) string {
	if n.Type == html.TextNode {
		return strings.TrimSpace(n.Data)
	}

	var text string
	for c := n.FirstChild; c != nil; c = c.NextSibling {
		text += extractTextContent(c)
	}
	return strings.TrimSpace(text)
}

// Get the value of a specific attribute from a node
func getAttrVal(n *html.Node, attrName string) string {
	for _, attr := range n.Attr {
		if attr.Key == attrName {
			return attr.Val
		}
	}
	return ""
}
