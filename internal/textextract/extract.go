package textextract

import (
	"archive/zip"
	"bytes"
	"encoding/xml"
	"fmt"
	"io"
	"strings"

	"github.com/ledongthuc/pdf"
)

// ExtractText attempts to read text from supported document types.
func ExtractText(filename string, data []byte) (string, error) {
	ext := strings.ToLower(strings.TrimPrefix(filepathExt(filename), "."))

	switch ext {
	case "txt":
		return string(data), nil
	case "pdf":
		return extractPDF(data)
	case "docx":
		return extractDocx(data)
	case "doc":
		return "", fmt.Errorf(".doc format is not supported; please convert to .docx")
	case "hwp":
		return "", fmt.Errorf(".hwp extraction is not yet supported")
	default:
		return "", fmt.Errorf("unsupported file type: %s", ext)
	}
}

func filepathExt(name string) string {
	for i := len(name) - 1; i >= 0; i-- {
		if name[i] == '.' {
			return name[i:]
		}
		if name[i] == '/' || name[i] == '\\' {
			break
		}
	}
	return ""
}

func extractPDF(data []byte) (string, error) {
	reader := bytes.NewReader(data)
	pdfReader, err := pdf.NewReader(reader, int64(len(data)))
	if err != nil {
		return "", fmt.Errorf("pdf parse failed: %w", err)
	}
	var buf strings.Builder
	numPages := pdfReader.NumPage()
	for i := 1; i <= numPages; i++ {
		page := pdfReader.Page(i)
		if page.V.IsNull() {
			continue
		}
		content, err := page.GetPlainText(nil)
		if err != nil {
			return "", fmt.Errorf("pdf text extraction failed: %w", err)
		}
		buf.WriteString(content)
		buf.WriteString("\n")
	}
	text := strings.TrimSpace(buf.String())
	if text == "" {
		return "", fmt.Errorf("pdf has no extractable text")
	}
	return text, nil
}

func extractDocx(data []byte) (string, error) {
	zr, err := zip.NewReader(bytes.NewReader(data), int64(len(data)))
	if err != nil {
		return "", fmt.Errorf("docx unzip 실패: %w", err)
	}

	var docXML io.Reader
	for _, f := range zr.File {
		if f.Name == "word/document.xml" {
			rc, err := f.Open()
			if err != nil {
				return "", err
			}
			defer rc.Close()
			docBuf, err := io.ReadAll(rc)
			if err != nil {
				return "", err
			}
			docXML = bytes.NewReader(docBuf)
			break
		}
	}

	if docXML == nil {
		return "", fmt.Errorf("docx에서 document.xml을 찾지 못했습니다")
	}

	decoder := xml.NewDecoder(docXML)
	var builder strings.Builder

	for {
		token, err := decoder.Token()
		if err == io.EOF {
			break
		}
		if err != nil {
			return "", fmt.Errorf("docx xml 파싱 실패: %w", err)
		}

		switch elem := token.(type) {
		case xml.StartElement:
			if elem.Name.Local == "t" {
				var content string
				if err := decoder.DecodeElement(&content, &elem); err != nil {
					return "", err
				}
				builder.WriteString(content)
			}
			if elem.Name.Local == "p" {
				builder.WriteString("\n")
			}
		}
	}

	text := strings.TrimSpace(builder.String())
	if text == "" {
		return "", fmt.Errorf("docx has no extractable text")
	}
	return text, nil
}
