package textextract

import (
	"archive/zip"
	"bytes"
	"encoding/xml"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	convertapi "github.com/ConvertAPI/convertapi-go/pkg"
	"github.com/ConvertAPI/convertapi-go/pkg/config"
	"github.com/ConvertAPI/convertapi-go/pkg/param"
	"github.com/pdfcpu/pdfcpu/pkg/api"
	"github.com/pdfcpu/pdfcpu/pkg/pdfcpu/model"
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
		return extractHWP(data)
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
	// Create temporary PDF file
	tmpPDF, err := os.CreateTemp("", "upload-*.pdf")
	if err != nil {
		return "", fmt.Errorf("pdf temp file create failed: %w", err)
	}
	defer os.Remove(tmpPDF.Name())

	if _, err := tmpPDF.Write(data); err != nil {
		tmpPDF.Close()
		return "", fmt.Errorf("pdf temp file write failed: %w", err)
	}
	if err := tmpPDF.Close(); err != nil {
		return "", fmt.Errorf("pdf temp file close failed: %w", err)
	}

	// 1) ConvertAPI 우선 시도
	if text, err := extractPDFViaConvertAPI(tmpPDF.Name()); err == nil && text != "" {
		return text, nil
	}

	// Create temporary output directory
	tmpDir, err := os.MkdirTemp("", "pdf-extract-*")
	if err != nil {
		return "", fmt.Errorf("temp dir create failed: %w", err)
	}
	defer os.RemoveAll(tmpDir)

	// Extract text using pdfcpu content extractor (text content only)
	conf := model.NewDefaultConfiguration()
	err = api.ExtractContentFile(tmpPDF.Name(), tmpDir, nil, conf)
	if err != nil {
		return "", fmt.Errorf("pdf text extraction failed: %w", err)
	}

	// Read extracted text files
	var builder strings.Builder
	entries, err := os.ReadDir(tmpDir)
	if err != nil {
		return "", fmt.Errorf("failed to read extracted content: %w", err)
	}

	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		filePath := filepath.Join(tmpDir, entry.Name())
		content, err := os.ReadFile(filePath)
		if err != nil {
			continue
		}
		builder.Write(content)
		builder.WriteString("\n")
	}

	text := strings.TrimSpace(builder.String())
	if text == "" {
		return "", fmt.Errorf("pdf has no extractable text")
	}
	return text, nil
}

// extractPDFViaConvertAPI uses ConvertAPI (requires CONVERTAPI_SECRET) to convert PDF to TXT.
func extractPDFViaConvertAPI(path string) (string, error) {
	secret := os.Getenv("CONVERTAPI_SECRET")
	if secret == "" {
		return "", fmt.Errorf("CONVERTAPI_SECRET is not set")
	}

	conf := config.NewDefault(secret)
	config.Default = conf

	params := []param.IParam{
		param.NewPath("File", path, conf),
	}

	result := convertapi.Convert("pdf", "txt", params, conf)
	files, err := result.Files()
	if err != nil {
		return "", fmt.Errorf("convertapi convert failed: %w", err)
	}
	if len(files) == 0 {
		return "", fmt.Errorf("convertapi returned no files")
	}

	var buf bytes.Buffer
	if _, err := io.Copy(&buf, files[0]); err != nil {
		return "", fmt.Errorf("convertapi read failed: %w", err)
	}

	text := strings.TrimSpace(buf.String())
	if text == "" {
		return "", fmt.Errorf("convertapi returned empty text")
	}
	return text, nil
}

func extractHWP(data []byte) (string, error) {
	tmp, err := os.CreateTemp("", "upload-*.hwp")
	if err != nil {
		return "", fmt.Errorf("hwp temp file create failed: %w", err)
	}
	defer os.Remove(tmp.Name())

	if _, err := tmp.Write(data); err != nil {
		tmp.Close()
		return "", fmt.Errorf("hwp temp file write failed: %w", err)
	}
	if err := tmp.Close(); err != nil {
		return "", fmt.Errorf("hwp temp file close failed: %w", err)
	}

	cmd := exec.Command("hwp5txt", tmp.Name())
	out, err := cmd.Output()
	if err != nil {
		return "", fmt.Errorf("hwp5txt execution failed: %w", err)
	}

	text := strings.TrimSpace(string(out))
	if text == "" {
		return "", fmt.Errorf("hwp 파일에서 텍스트를 추출하지 못했습니다")
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
