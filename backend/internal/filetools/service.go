package filetools

import (
	"archive/zip"
	"bytes"
	"context"
	"errors"
	"fmt"
	"image"
	_ "image/gif"
	_ "image/jpeg"
	_ "image/png"
	"io"
	"mime"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"unicode"

	"github.com/gpdf-dev/gpdf"
	"github.com/gpdf-dev/gpdf/document"
	"github.com/gpdf-dev/gpdf/template"
)

var (
	ErrInvalid     = errors.New("invalid file tool input")
	ErrUnavailable = errors.New("file tool unavailable")
	ErrConversion  = errors.New("file conversion failed")
)

const (
	maxFiles       = 12
	maxInputBytes  = 64 << 20
	maxOutputBytes = 40 << 20
)

type Input struct {
	Name        string
	ContentType string
	Data        []byte
}

type Options struct {
	PageRange   string
	Quality     string
	ImageFormat string
	MaxWidth    int
}

type Option struct {
	ID          string   `json:"id"`
	Label       string   `json:"label"`
	Type        string   `json:"type"`
	Placeholder string   `json:"placeholder,omitempty"`
	Default     any      `json:"default,omitempty"`
	Choices     []Choice `json:"choices,omitempty"`
}

type Choice struct {
	Label string `json:"label"`
	Value string `json:"value"`
}

type Definition struct {
	ID          string   `json:"id"`
	Name        string   `json:"name"`
	Description string   `json:"description"`
	Category    string   `json:"category"`
	Icon        string   `json:"icon"`
	Accept      string   `json:"accept"`
	Multiple    bool     `json:"multiple"`
	MinFiles    int      `json:"minFiles"`
	MaxFiles    int      `json:"maxFiles"`
	Available   bool     `json:"available"`
	Unavailable string   `json:"unavailableReason,omitempty"`
	Options     []Option `json:"options,omitempty"`
	requires    []string
}

type Result struct {
	Name        string
	ContentType string
	Data        []byte
	Summary     string
}

type Service struct {
	definitions []Definition
	python      string
	script      string
	slots       chan struct{}
}

var pageRangePattern = regexp.MustCompile(`^\d+(?:-\d+)?(?:,\d+(?:-\d+)?)*$`)

func New() *Service {
	python := strings.TrimSpace(os.Getenv("AI_WORKBENCH_PDF_TO_WORD_PYTHON"))
	if python == "" {
		python = "/opt/ai-workbench-filetools/bin/python"
	}
	script := strings.TrimSpace(os.Getenv("AI_WORKBENCH_PDF_TO_WORD_SCRIPT"))
	if script == "" {
		script = "/app/scripts/pdf_to_word.py"
	}
	qualityChoices := []Choice{{Label: "屏幕阅读（最小）", Value: "screen"}, {Label: "日常分享（推荐）", Value: "ebook"}, {Label: "打印（清晰）", Value: "printer"}}
	imageChoices := []Choice{{Label: "JPG", Value: "jpg"}, {Label: "PNG", Value: "png"}, {Label: "WebP", Value: "webp"}}
	return &Service{python: python, script: script, slots: make(chan struct{}, 2), definitions: []Definition{
		{ID: "office_to_pdf", Name: "Word / Office 转 PDF", Description: "把 Word、Excel、PPT 和 OpenDocument 转成 PDF", Category: "文档转换", Icon: "Document", Accept: ".doc,.docx,.xls,.xlsx,.ppt,.pptx,.odt,.ods,.odp", MinFiles: 1, MaxFiles: 1, requires: []string{"libreoffice"}},
		{ID: "pdf_to_word", Name: "PDF 转 Word", Description: "将 PDF 的文字、表格和图片转换为可编辑 DOCX", Category: "文档转换", Icon: "DocumentCopy", Accept: ".pdf,application/pdf", MinFiles: 1, MaxFiles: 1, requires: []string{"pdf-to-word"}},
		{ID: "merge_pdf", Name: "合并 PDF", Description: "按选择顺序把多个 PDF 合并成一个文件", Category: "PDF 工具", Icon: "Files", Accept: ".pdf,application/pdf", Multiple: true, MinFiles: 2, MaxFiles: 12, requires: []string{"qpdf"}},
		{ID: "extract_pdf_pages", Name: "提取 PDF 页面", Description: "按页码范围生成新的 PDF，例如 1-3,5,8-10", Category: "PDF 工具", Icon: "Scissor", Accept: ".pdf,application/pdf", MinFiles: 1, MaxFiles: 1, requires: []string{"qpdf"}, Options: []Option{{ID: "pageRange", Label: "页码范围", Type: "text", Placeholder: "例如：1-3,5", Default: "1"}}},
		{ID: "compress_pdf", Name: "压缩 PDF", Description: "为手机分享、日常阅读或打印压缩 PDF", Category: "PDF 工具", Icon: "Download", Accept: ".pdf,application/pdf", MinFiles: 1, MaxFiles: 1, requires: []string{"gs"}, Options: []Option{{ID: "quality", Label: "压缩质量", Type: "select", Default: "ebook", Choices: qualityChoices}}},
		{ID: "pdf_to_images", Name: "PDF 转图片", Description: "把每一页转换成 JPG，并打包为 ZIP 下载", Category: "PDF 工具", Icon: "Picture", Accept: ".pdf,application/pdf", MinFiles: 1, MaxFiles: 1, requires: []string{"pdftoppm"}},
		{ID: "pdf_to_text", Name: "PDF 提取文字", Description: "提取 PDF 中可搜索的文字，生成 TXT", Category: "PDF 工具", Icon: "Tickets", Accept: ".pdf,application/pdf", MinFiles: 1, MaxFiles: 1, requires: []string{"pdftotext"}},
		{ID: "images_to_pdf", Name: "图片合并为 PDF", Description: "按选择顺序将 JPG、PNG 图片排版到 A4 PDF", Category: "图片工具", Icon: "Collection", Accept: "image/jpeg,image/png,.jpg,.jpeg,.png", Multiple: true, MinFiles: 1, MaxFiles: 12},
		{ID: "convert_images", Name: "图片转换与压缩", Description: "转换 JPG、PNG、WebP，可限制宽度并压缩体积", Category: "图片工具", Icon: "MagicStick", Accept: "image/jpeg,image/png,image/webp,.jpg,.jpeg,.png,.webp", Multiple: true, MinFiles: 1, MaxFiles: 12, requires: []string{"imagemagick"}, Options: []Option{{ID: "imageFormat", Label: "输出格式", Type: "select", Default: "jpg", Choices: imageChoices}, {ID: "quality", Label: "图片质量", Type: "number", Default: 82}, {ID: "maxWidth", Label: "最大宽度（像素，0 为原尺寸）", Type: "number", Default: 1920}}},
		{ID: "zip_files", Name: "文件打包 ZIP", Description: "把多个文件快速打包，方便手机发送和归档", Category: "办公整理", Icon: "Folder", Accept: "*/*", Multiple: true, MinFiles: 1, MaxFiles: 12},
	}}
}

func (s *Service) Catalog() []Definition {
	result := make([]Definition, len(s.definitions))
	for index, definition := range s.definitions {
		definition.Available = true
		definition.Unavailable = ""
		for _, requirement := range definition.requires {
			if !s.available(requirement) {
				definition.Available = false
				definition.Unavailable = "服务正在安装该转换组件"
				break
			}
		}
		definition.requires = nil
		result[index] = definition
	}
	return result
}

func (s *Service) Run(ctx context.Context, operation string, inputs []Input, options Options) (*Result, error) {
	definition, ok := s.definition(operation)
	if !ok || len(inputs) < definition.MinFiles || len(inputs) > definition.MaxFiles || len(inputs) > maxFiles {
		return nil, ErrInvalid
	}
	for _, requirement := range definition.requires {
		if !s.available(requirement) {
			return nil, ErrUnavailable
		}
	}
	total := 0
	for _, input := range inputs {
		if len(input.Data) == 0 || len(input.Data) > maxInputBytes {
			return nil, ErrInvalid
		}
		total += len(input.Data)
		if total > maxInputBytes || !accepts(operation, input.Name) {
			return nil, ErrInvalid
		}
	}
	select {
	case s.slots <- struct{}{}:
		defer func() { <-s.slots }()
	case <-ctx.Done():
		return nil, ctx.Err()
	}

	workingDirectory, err := os.MkdirTemp("", "ai-workbench-file-tool-*")
	if err != nil {
		return nil, err
	}
	defer os.RemoveAll(workingDirectory)
	paths := make([]string, 0, len(inputs))
	for index, input := range inputs {
		path := filepath.Join(workingDirectory, fmt.Sprintf("%02d-%s", index+1, safeName(input.Name)))
		if err := os.WriteFile(path, input.Data, 0o600); err != nil {
			return nil, err
		}
		paths = append(paths, path)
	}

	var result *Result
	switch operation {
	case "office_to_pdf":
		result, err = s.officeToPDF(ctx, workingDirectory, paths[0], inputs[0].Name)
	case "pdf_to_word":
		result, err = s.pdfToWord(ctx, workingDirectory, paths[0], inputs[0].Name)
	case "merge_pdf":
		result, err = s.mergePDF(ctx, workingDirectory, paths)
	case "extract_pdf_pages":
		result, err = s.extractPDFPages(ctx, workingDirectory, paths[0], inputs[0].Name, options.PageRange)
	case "compress_pdf":
		result, err = s.compressPDF(ctx, workingDirectory, paths[0], inputs[0].Name, options.Quality)
	case "pdf_to_images":
		result, err = s.pdfToImages(ctx, workingDirectory, paths[0], inputs[0].Name)
	case "pdf_to_text":
		result, err = s.pdfToText(ctx, workingDirectory, paths[0], inputs[0].Name)
	case "images_to_pdf":
		result, err = imagesToPDF(inputs)
	case "convert_images":
		result, err = s.convertImages(ctx, workingDirectory, paths, inputs, options)
	case "zip_files":
		result, err = zipInputs("文件打包.zip", inputs)
	default:
		return nil, ErrInvalid
	}
	if err != nil {
		return nil, err
	}
	if result == nil || len(result.Data) == 0 || len(result.Data) > maxOutputBytes {
		return nil, ErrConversion
	}
	return result, nil
}

func (s *Service) definition(id string) (Definition, bool) {
	for _, definition := range s.definitions {
		if definition.ID == id {
			return definition, true
		}
	}
	return Definition{}, false
}

func (s *Service) available(requirement string) bool {
	switch requirement {
	case "pdf-to-word":
		return executableFile(s.python) && regularFile(s.script)
	case "imagemagick":
		return commandPath("magick") != "" || commandPath("convert") != ""
	default:
		return commandPath(requirement) != ""
	}
}

func (s *Service) officeToPDF(ctx context.Context, directory, inputPath, originalName string) (*Result, error) {
	outputDirectory := filepath.Join(directory, "office-output")
	profileDirectory := filepath.Join(directory, "libreoffice-profile")
	if err := os.MkdirAll(outputDirectory, 0o700); err != nil {
		return nil, err
	}
	profileURL := "file://" + filepath.ToSlash(profileDirectory)
	if err := runCommand(ctx, "libreoffice", "-env:UserInstallation="+profileURL, "--headless", "--convert-to", "pdf", "--outdir", outputDirectory, inputPath); err != nil {
		return nil, err
	}
	matches, _ := filepath.Glob(filepath.Join(outputDirectory, "*.pdf"))
	if len(matches) != 1 {
		return nil, ErrConversion
	}
	return fileResult(replaceExtension(originalName, ".pdf"), "application/pdf", matches[0], "文档已转换为 PDF")
}

func (s *Service) pdfToWord(ctx context.Context, directory, inputPath, originalName string) (*Result, error) {
	outputPath := filepath.Join(directory, "converted.docx")
	if err := runCommand(ctx, s.python, s.script, inputPath, outputPath); err != nil {
		return nil, err
	}
	return fileResult(replaceExtension(originalName, ".docx"), "application/vnd.openxmlformats-officedocument.wordprocessingml.document", outputPath, "PDF 已转换为可编辑 Word 文档")
}

func (s *Service) mergePDF(ctx context.Context, directory string, paths []string) (*Result, error) {
	outputPath := filepath.Join(directory, "merged.pdf")
	arguments := []string{"--empty", "--pages"}
	arguments = append(arguments, paths...)
	arguments = append(arguments, "--", outputPath)
	if err := runCommand(ctx, "qpdf", arguments...); err != nil {
		return nil, err
	}
	return fileResult("合并文档.pdf", "application/pdf", outputPath, fmt.Sprintf("已按顺序合并 %d 个 PDF", len(paths)))
}

func (s *Service) extractPDFPages(ctx context.Context, directory, inputPath, originalName, pageRange string) (*Result, error) {
	pageRange = strings.ReplaceAll(strings.TrimSpace(pageRange), " ", "")
	if !pageRangePattern.MatchString(pageRange) {
		return nil, ErrInvalid
	}
	outputPath := filepath.Join(directory, "pages.pdf")
	if err := runCommand(ctx, "qpdf", inputPath, "--pages", ".", pageRange, "--", outputPath); err != nil {
		return nil, err
	}
	return fileResult(withSuffix(originalName, "-选定页面", ".pdf"), "application/pdf", outputPath, "已提取页面 "+pageRange)
}

func (s *Service) compressPDF(ctx context.Context, directory, inputPath, originalName, quality string) (*Result, error) {
	quality = strings.TrimSpace(quality)
	if quality == "" {
		quality = "ebook"
	}
	if quality != "screen" && quality != "ebook" && quality != "printer" {
		return nil, ErrInvalid
	}
	outputPath := filepath.Join(directory, "compressed.pdf")
	if err := runCommand(ctx, "gs", "-sDEVICE=pdfwrite", "-dCompatibilityLevel=1.4", "-dPDFSETTINGS=/"+quality, "-dNOPAUSE", "-dQUIET", "-dBATCH", "-sOutputFile="+outputPath, inputPath); err != nil {
		return nil, err
	}
	return fileResult(withSuffix(originalName, "-压缩", ".pdf"), "application/pdf", outputPath, "PDF 已压缩，原文件不会被修改")
}

func (s *Service) pdfToImages(ctx context.Context, directory, inputPath, originalName string) (*Result, error) {
	outputDirectory := filepath.Join(directory, "pages")
	if err := os.MkdirAll(outputDirectory, 0o700); err != nil {
		return nil, err
	}
	prefix := filepath.Join(outputDirectory, "page")
	if err := runCommand(ctx, "pdftoppm", "-jpeg", "-jpegopt", "quality=88", "-r", "150", inputPath, prefix); err != nil {
		return nil, err
	}
	paths, _ := filepath.Glob(prefix + "-*.jpg")
	sort.Strings(paths)
	if len(paths) == 0 {
		return nil, ErrConversion
	}
	inputs := make([]Input, 0, len(paths))
	for _, path := range paths {
		data, err := os.ReadFile(path)
		if err != nil {
			return nil, err
		}
		inputs = append(inputs, Input{Name: filepath.Base(path), ContentType: "image/jpeg", Data: data})
	}
	return zipInputs(withSuffix(originalName, "-图片", ".zip"), inputs)
}

func (s *Service) pdfToText(ctx context.Context, directory, inputPath, originalName string) (*Result, error) {
	outputPath := filepath.Join(directory, "content.txt")
	if err := runCommand(ctx, "pdftotext", "-layout", "-enc", "UTF-8", inputPath, outputPath); err != nil {
		return nil, err
	}
	return fileResult(replaceExtension(originalName, ".txt"), "text/plain; charset=utf-8", outputPath, "已提取 PDF 中可搜索的文字")
}

func imagesToPDF(inputs []Input) (*Result, error) {
	doc := gpdf.NewDocument(gpdf.WithPageSize(gpdf.A4), gpdf.WithMargins(document.UniformEdges(document.Mm(10))))
	for _, input := range inputs {
		configuration, _, err := image.DecodeConfig(bytes.NewReader(input.Data))
		if err != nil || configuration.Width <= 0 || configuration.Height <= 0 || configuration.Width > 12000 || configuration.Height > 12000 || int64(configuration.Width)*int64(configuration.Height) > 60_000_000 {
			return nil, ErrInvalid
		}
		page := doc.AddPage()
		page.AutoRow(func(row *template.RowBuilder) {
			row.Col(12, func(column *template.ColBuilder) {
				if float64(configuration.Width)/float64(configuration.Height) >= 190.0/277.0 {
					column.Image(input.Data, template.FitWidth(document.Mm(190)))
				} else {
					column.Image(input.Data, template.FitHeight(document.Mm(277)))
				}
			})
		})
	}
	data, err := doc.Generate()
	if err != nil {
		return nil, ErrConversion
	}
	return &Result{Name: "图片合并.pdf", ContentType: "application/pdf", Data: data, Summary: fmt.Sprintf("已按顺序将 %d 张图片合并为 PDF", len(inputs))}, nil
}

func (s *Service) convertImages(ctx context.Context, directory string, paths []string, originals []Input, options Options) (*Result, error) {
	format := strings.ToLower(strings.TrimPrefix(strings.TrimSpace(options.ImageFormat), "."))
	if format == "jpeg" {
		format = "jpg"
	}
	if format == "" {
		format = "jpg"
	}
	if format != "jpg" && format != "png" && format != "webp" {
		return nil, ErrInvalid
	}
	quality, err := strconv.Atoi(strings.TrimSpace(options.Quality))
	if err != nil || quality == 0 {
		quality = 82
	}
	if quality < 20 || quality > 100 || options.MaxWidth < 0 || options.MaxWidth > 10000 {
		return nil, ErrInvalid
	}
	imageCommand := commandPath("magick")
	if imageCommand == "" {
		imageCommand = commandPath("convert")
	}
	converted := make([]Input, 0, len(paths))
	for index, inputPath := range paths {
		name := withSuffix(originals[index].Name, "-已转换", "."+format)
		outputPath := filepath.Join(directory, fmt.Sprintf("converted-%02d.%s", index+1, format))
		arguments := []string{"-limit", "memory", "256MiB", "-limit", "map", "512MiB", "-limit", "disk", "1GiB", inputPath, "-auto-orient", "-strip"}
		if options.MaxWidth > 0 {
			arguments = append(arguments, "-resize", fmt.Sprintf("%dx>", options.MaxWidth))
		}
		arguments = append(arguments, "-quality", strconv.Itoa(quality), outputPath)
		if err := runCommand(ctx, imageCommand, arguments...); err != nil {
			return nil, err
		}
		data, err := readLimited(outputPath)
		if err != nil {
			return nil, err
		}
		converted = append(converted, Input{Name: name, ContentType: imageContentType(format), Data: data})
	}
	if len(converted) == 1 {
		return &Result{Name: converted[0].Name, ContentType: converted[0].ContentType, Data: converted[0].Data, Summary: "图片已转换并压缩"}, nil
	}
	return zipInputs("转换后的图片.zip", converted)
}

func zipInputs(name string, inputs []Input) (*Result, error) {
	var output bytes.Buffer
	writer := zip.NewWriter(&output)
	used := map[string]int{}
	for _, input := range inputs {
		name := uniqueName(safeName(input.Name), used)
		header := &zip.FileHeader{Name: name, Method: zip.Deflate}
		header.SetMode(0o600)
		entry, err := writer.CreateHeader(header)
		if err != nil {
			return nil, err
		}
		if _, err := entry.Write(input.Data); err != nil {
			return nil, err
		}
		if output.Len() > maxOutputBytes {
			return nil, ErrConversion
		}
	}
	if err := writer.Close(); err != nil {
		return nil, err
	}
	return &Result{Name: name, ContentType: "application/zip", Data: output.Bytes(), Summary: fmt.Sprintf("已打包 %d 个文件", len(inputs))}, nil
}

func fileResult(name, contentType, path, summary string) (*Result, error) {
	data, err := readLimited(path)
	if err != nil {
		return nil, err
	}
	return &Result{Name: name, ContentType: contentType, Data: data, Summary: summary}, nil
}

func readLimited(path string) ([]byte, error) {
	file, err := os.Open(path)
	if err != nil {
		return nil, ErrConversion
	}
	defer file.Close()
	data, err := io.ReadAll(io.LimitReader(file, maxOutputBytes+1))
	if err != nil || len(data) == 0 || len(data) > maxOutputBytes {
		return nil, ErrConversion
	}
	return data, nil
}

func runCommand(ctx context.Context, name string, arguments ...string) error {
	command := exec.CommandContext(ctx, name, arguments...)
	var stderr bytes.Buffer
	command.Stdout = io.Discard
	command.Stderr = &limitedWriter{writer: &stderr, remaining: 4096}
	if err := command.Run(); err != nil {
		if ctx.Err() != nil {
			return ctx.Err()
		}
		return fmt.Errorf("%w: %s", ErrConversion, compactError(stderr.String()))
	}
	return nil
}

type limitedWriter struct {
	writer    io.Writer
	remaining int
}

func (w *limitedWriter) Write(data []byte) (int, error) {
	original := len(data)
	if len(data) > w.remaining {
		data = data[:w.remaining]
	}
	if len(data) > 0 {
		_, _ = w.writer.Write(data)
		w.remaining -= len(data)
	}
	return original, nil
}

func compactError(value string) string {
	value = strings.Join(strings.Fields(value), " ")
	if value == "" {
		return "转换组件未能处理该文件"
	}
	if len([]rune(value)) > 180 {
		return string([]rune(value)[:180])
	}
	return value
}

func accepts(operation, name string) bool {
	extension := strings.ToLower(filepath.Ext(strings.TrimSpace(name)))
	switch operation {
	case "office_to_pdf":
		return contains([]string{".doc", ".docx", ".xls", ".xlsx", ".ppt", ".pptx", ".odt", ".ods", ".odp"}, extension)
	case "pdf_to_word", "merge_pdf", "extract_pdf_pages", "compress_pdf", "pdf_to_images", "pdf_to_text":
		return extension == ".pdf"
	case "images_to_pdf":
		return extension == ".jpg" || extension == ".jpeg" || extension == ".png"
	case "convert_images":
		return extension == ".jpg" || extension == ".jpeg" || extension == ".png" || extension == ".webp"
	case "zip_files":
		return strings.TrimSpace(filepath.Base(name)) != ""
	default:
		return false
	}
}

func safeName(name string) string {
	name = strings.TrimSpace(filepath.Base(name))
	if name == "" || name == "." {
		return "file"
	}
	var builder strings.Builder
	for _, character := range []rune(name) {
		if unicode.IsLetter(character) || unicode.IsDigit(character) || strings.ContainsRune(" ._-()[]", character) {
			builder.WriteRune(character)
		} else {
			builder.WriteRune('_')
		}
		if builder.Len() >= 180 {
			break
		}
	}
	result := strings.Trim(builder.String(), " .")
	if result == "" {
		return "file"
	}
	return result
}

func uniqueName(name string, used map[string]int) string {
	key := strings.ToLower(name)
	used[key]++
	if used[key] == 1 {
		return name
	}
	extension := filepath.Ext(name)
	return strings.TrimSuffix(name, extension) + "-" + strconv.Itoa(used[key]) + extension
}

func replaceExtension(name, extension string) string {
	name = safeName(name)
	return strings.TrimSuffix(name, filepath.Ext(name)) + extension
}

func withSuffix(name, suffix, extension string) string {
	name = safeName(name)
	return strings.TrimSuffix(name, filepath.Ext(name)) + suffix + extension
}

func regularFile(path string) bool {
	info, err := os.Stat(path)
	return err == nil && info.Mode().IsRegular()
}

func executableFile(path string) bool {
	info, err := os.Stat(path)
	return err == nil && info.Mode().IsRegular() && info.Mode().Perm()&0o111 != 0
}

func commandPath(name string) string {
	path, _ := exec.LookPath(name)
	return path
}

func imageContentType(format string) string {
	if format == "jpg" {
		return "image/jpeg"
	}
	if value := mime.TypeByExtension("." + format); value != "" {
		return value
	}
	return "application/octet-stream"
}

func contains(values []string, target string) bool {
	for _, value := range values {
		if value == target {
			return true
		}
	}
	return false
}
