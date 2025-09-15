package main

import (
	"bufio"
	"encoding/csv"
	"flag"
	"fmt"
	"html"
	"io"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

// Configと他の補助関数 (processFile, findCsvFiles, etc.) は変更ありません
// Config はアプリケーションの設定を保持します。
type Config struct {
	InputPath    string
	Columns      []string
	SearchTarget string
	Recursive    bool
	OutFile      string
	AfterOpen    bool
	FontName     string
}

// processFile は単一のCSVファイルを処理し、HTML形式でwriterに出力します。
func processFile(filePath string, cfg Config, writer io.Writer) error {
	file, err := os.Open(filePath)
	if err != nil {
		return fmt.Errorf("failed to open file: %w", err)
	}
	defer file.Close()
	reader := csv.NewReader(bufio.NewReader(file))
	reader.ReuseRecord = true
	headers, err := reader.Read()
	if err == io.EOF {
		return nil
	}
	if err != nil {
		return fmt.Errorf("failed to read headers: %w", err)
	}
	headerMap := make(map[string]int, len(headers))
	for i, h := range headers {
		headerMap[h] = i
	}
	targetIndices := make([]int, 0, len(cfg.Columns))
	targetColumns := make([]string, 0, len(cfg.Columns))
	for _, col := range cfg.Columns {
		if idx, ok := headerMap[col]; ok {
			targetIndices = append(targetIndices, idx)
			targetColumns = append(targetColumns, col)
		} else {
			log.Printf("Warning: Column '%s' not found in %s", col, filePath)
		}
	}
	if len(targetIndices) == 0 {
		log.Printf("Warning: None of the specified columns found in %s. Skipping file.", filePath)
		return nil
	}
	lineNum := 1
	for {
		lineNum++
		record, err := reader.Read()
		if err == io.EOF {
			break
		}
		if err != nil {
			log.Printf("Warning: Parse error in %s at line %d: %v", filePath, lineNum, err)
			continue
		}
		if cfg.SearchTarget != "" {
			found := false
			for _, cell := range record {
				if strings.Contains(cell, cfg.SearchTarget) {
					found = true
					break
				}
			}
			if !found {
				continue
			}
		}
		var sb strings.Builder
		fmt.Fprintln(&sb, "<div class=\"record\">")
		fmt.Fprintf(&sb, "  <p class=\"file-info\">--- File: %s, Line: %d ---</p>\n", html.EscapeString(filePath), lineNum)
		for i, colName := range targetColumns {
			idx := targetIndices[i]
			if idx < len(record) {
				key := html.EscapeString(colName)
				value := html.EscapeString(record[idx])
				fmt.Fprintf(&sb, "  <p><span class=\"header\">%s:</span><span class=\"value\">[%s]</span></p>\n", key, value)
			}
		}
		fmt.Fprintln(&sb, "</div>")

		if _, err := fmt.Fprint(writer, sb.String()); err != nil {
			return fmt.Errorf("failed to write to output: %w", err)
		}
	}
	return nil
}

// writeHtmlHeader はHTMLのヘッダーとCSSスタイルを出力します
func writeHtmlHeader(writer io.Writer, fontName string) {
	// フォントが指定されている場合のみ、.valueセレクタにfont-familyスタイルを追加する
	valueFontStyle := ""
	if fontName != "" {
		// CSSインジェクションを防ぐため、フォント名をエスケープする
		escapedFontName := html.EscapeString(fontName)
		valueFontStyle = fmt.Sprintf(`font-family: "%s", sans-serif;`, escapedFontName)
	}

	header := fmt.Sprintf(`<!DOCTYPE html>
<html lang="ja">
<head>
  <meta charset="UTF-8">
  <meta name="viewport" content="width=device-width, initial-scale=1.0">
  <title>CSV Extract Result</title>
  <style>
    body {
      /* UI部分は標準的なフォントに固定 */
      font-family: -apple-system, BlinkMacSystemFont, "Segoe UI", Roboto, "Helvetica Neue", Arial, sans-serif;
      background-color: #f4f4f9;
      color: #333;
      margin: 0;
      padding: 20px;
    }
    .record {
      background-color: #fff;
      border: 1px solid #ddd;
      border-radius: 8px;
      padding: 15px;
      margin-bottom: 15px;
      box-shadow: 0 2px 4px rgba(0,0,0,0.1);
    }
    .file-info {
      font-size: 0.9em;
      color: #666;
      border-bottom: 1px solid #eee;
      padding-bottom: 10px;
      margin-top: 0;
    }
    .header {
      color: #007bff; /* Cyan/Blue */
      font-weight: bold;
    }
    .value {
      color: #28a745; /* Green */
      /* ここに、-fontで指定されたフォントスタイルが挿入される */
      %s
    }
  </style>
</head>
<body>
  <h1>CSV Extract Result</h1>
`, valueFontStyle)
	fmt.Fprint(writer, header)
}

// writeHtmlFooter はHTMLのフッターを出力します
func writeHtmlFooter(writer io.Writer) {
	footer := `
</body>
</html>
`
	fmt.Fprint(writer, footer)
}

func findCsvFiles(root string, recursive bool) ([]string, error) {
	var files []string
	info, err := os.Stat(root)
	if err != nil {
		return nil, fmt.Errorf("could not stat path %s: %w", root, err)
	}
	if !info.IsDir() {
		if strings.HasSuffix(strings.ToLower(root), ".csv") {
			return []string{root}, nil
		}
		return files, nil
	}
	walkFunc := func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if !d.IsDir() && strings.HasSuffix(strings.ToLower(d.Name()), ".csv") {
			files = append(files, path)
		}
		return nil
	}
	if recursive {
		if err := filepath.WalkDir(root, walkFunc); err != nil {
			return nil, fmt.Errorf("error walking directory %s: %w", root, err)
		}
	} else {
		entries, err := os.ReadDir(root)
		if err != nil {
			return nil, fmt.Errorf("error reading directory %s: %w", root, err)
		}
		for _, entry := range entries {
			if err := walkFunc(filepath.Join(root, entry.Name()), entry, nil); err != nil {
				log.Printf("Warning: could not process entry %s: %v", entry.Name(), err)
			}
		}
	}
	return files, nil
}

// parseFlags はコマンドライン引数を解析し、設定を構成します。
func parseFlags() Config {
	var cfg Config
	var columnsStr string
	flag.StringVar(&cfg.InputPath, "in", "", "Path to the CSV file or directory.")
	flag.StringVar(&columnsStr, "cols", "", "Comma-separated list of column names to extract.")
	flag.StringVar(&cfg.SearchTarget, "target", "", "A string to filter lines by.")
	flag.StringVar(&cfg.OutFile, "out", "", "Path to the output HTML file (e.g., results.html).")
	flag.StringVar(&cfg.FontName, "font", "", "Font name to apply to the value part in the HTML file (optional).")
	flag.BoolVar(&cfg.Recursive, "r", false, "Search for CSV files recursively in subdirectories.")
	flag.BoolVar(&cfg.AfterOpen, "after-open", false, "Open the output file after processing (requires -out).")

	flag.Usage = func() {
		fmt.Fprintf(os.Stderr, "Usage: %s -in <path> -cols <col1,col2> -out <file.html> [options]\n", os.Args[0])
		fmt.Fprintln(os.Stderr, "\nExample: go-ChiiCgrep.exe -in data -cols Name,Email -target 東京 -out results.html -font \"MS Mincho\" -after-open")
		fmt.Fprintln(os.Stderr, "\nOptions:")
		flag.PrintDefaults()
	}

	flag.Parse()

	if cfg.InputPath == "" || columnsStr == "" {
		flag.Usage()
		os.Exit(1)
	}
	cfg.Columns = strings.Split(columnsStr, ",")
	return cfg
}

// openFile は指定されたファイルをWindowsのデフォルトアプリケーションで開きます。
func openFile(path string) error {
	cmd := exec.Command("cmd", "/c", "start", "", path)
	return cmd.Run()
}

func main() {
	log.SetFlags(0)
	cfg := parseFlags()

	var outputWriter io.Writer = os.Stdout
	var outFile *os.File
	var err error

	if cfg.OutFile != "" {
		outFile, err = os.Create(cfg.OutFile)
		if err != nil {
			log.Fatalf("Error: could not create output file %s: %v", cfg.OutFile, err)
		}
		outputWriter = outFile
		writeHtmlHeader(outputWriter, cfg.FontName)
	} else {
		log.Println("Warning: Outputting HTML to console. For best results, use the -out flag to save as an .html file.")
	}

	files, err := findCsvFiles(cfg.InputPath, cfg.Recursive)
	if err != nil {
		log.Fatalf("Error: %v", err)
	}

	if len(files) == 0 {
		log.Println("No CSV files found.")
		return
	}

	for _, file := range files {
		if err := processFile(file, cfg, outputWriter); err != nil {
			log.Printf("Error processing %s: %v", file, err)
		}
	}

	if outFile != nil {
		writeHtmlFooter(outputWriter)
		outFile.Close()
	}

	if cfg.AfterOpen && cfg.OutFile != "" {
		absPath, err := filepath.Abs(cfg.OutFile)
		if err != nil {
			log.Printf("Error: could not determine absolute path for %s: %v", cfg.OutFile, err)
			return
		}

		fmt.Fprintf(os.Stderr, "Processing complete. Opening %s...\n", absPath)
		if err := openFile(absPath); err != nil {
			log.Printf("Error: could not open output file %s: %v", absPath, err)
		}
	}
}
