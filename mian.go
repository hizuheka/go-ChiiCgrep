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

// ColumnSpec は、列名とそれが強調表示されるべきかどうかの情報を保持します。
type ColumnSpec struct {
	Name      string
	Emphasize bool
}

// Config は、アプリケーションのすべての設定を保持します。
type Config struct {
	InputPath    string
	Columns      []ColumnSpec // 抽出する列の仕様
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
		return fmt.Errorf("ファイルを開けませんでした: %w", err)
	}
	defer file.Close()

	reader := csv.NewReader(bufio.NewReader(file))
	reader.ReuseRecord = true

	headers, err := reader.Read()
	if err == io.EOF {
		return nil
	}
	if err != nil {
		return fmt.Errorf("ヘッダーの読み込みに失敗しました: %w", err)
	}

	headerMap := make(map[string]int, len(headers))
	for i, h := range headers {
		headerMap[h] = i
	}

	// 処理対象の列情報を効率的に扱うための内部構造体
	type targetColumn struct {
		Name      string
		Index     int
		Emphasize bool
	}

	var targetColumns []targetColumn
	for _, spec := range cfg.Columns {
		if idx, ok := headerMap[spec.Name]; ok {
			targetColumns = append(targetColumns, targetColumn{
				Name:      spec.Name,
				Index:     idx,
				Emphasize: spec.Emphasize,
			})
		} else {
			log.Printf("警告: 列 '%s' がファイル %s に見つかりません", spec.Name, filePath)
		}
	}

	if len(targetColumns) == 0 {
		log.Printf("警告: 指定された列が %s に見つかりませんでした。このファイルをスキップします。", filePath)
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
			log.Printf("警告: %s の %d行目で解析エラーが発生しました: %v", filePath, lineNum, err)
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
		fmt.Fprintf(&sb, "  <p class=\"file-info\">--- ファイル: %s, 行: %d ---</p>\n", html.EscapeString(filePath), lineNum)

		for _, col := range targetColumns {
			idx := col.Index
			if idx < len(record) {
				key := html.EscapeString(col.Name)
				value := html.EscapeString(record[idx])
				className := "data-item"
				if col.Emphasize {
					className += " emphasis" // 強調フラグがあればemphasisクラスを追加
				}
				fmt.Fprintf(&sb, "  <p class=\"%s\"><span class=\"header\">%s: </span><span class=\"value\">[%s]</span></p>\n", className, key, value)
			}
		}
		fmt.Fprintln(&sb, "</div>")

		if _, err := fmt.Fprint(writer, sb.String()); err != nil {
			return fmt.Errorf("出力への書き込みに失敗しました: %w", err)
		}
	}
	return nil
}

// writeHtmlHeader はHTMLのヘッダーとCSSスタイルを出力します
func writeHtmlHeader(writer io.Writer, fontName string) {
	valueFontStyle := ""
	if fontName != "" {
		escapedFontName := html.EscapeString(fontName)
		valueFontStyle = fmt.Sprintf(`font-family: "%s", sans-serif;`, escapedFontName)
	}

	header := fmt.Sprintf(`<!DOCTYPE html>
<html lang="ja">
<head>
  <meta charset="UTF-8">
  <meta name="viewport" content="width=device-width, initial-scale=1.0">
  <title>CSV抽出結果</title>
  <style>
    body {
      font-family: -apple-system, BlinkMacSystemFont, "Segoe UI", Roboto, "Helvetica Neue", Arial, sans-serif;
      background-color: #f4f4f9;
      color: #333;
      margin: 0;
      padding: 20px;
    }
    h1 {
      font-size: 1.5em;
      margin-top: 0;
      margin-bottom: 12px;
      padding-bottom: 8px;
      border-bottom: 1px solid #ccc;
    }
    .record {
      background-color: #fff;
      border: 1px solid #ddd;
      border-radius: 8px;
      padding: 15px;
      margin-bottom: 15px;
      box-shadow: 0 2px 4px rgba(0,0,0,0.1);
    }
    .data-item {
      margin-top: 0;
      margin-bottom: 0;
      padding: 2px 4px;
      border-radius: 3px;
    }
    .emphasis {
      font-weight: bold;
      background-color: #fff8c4; /* 薄い黄色のハイライト */
    }
    .file-info {
      font-size: 0.9em;
      color: #666;
      border-bottom: 1px solid #eee;
      padding-bottom: 10px;
      margin-top: 0;
      margin-bottom: 8px;
    }
    .header {
      color: #007bff;
      font-weight: bold;
    }
    .value {
      color: #28a745;
      %s
    }
  </style>
</head>
<body>
  <h1>CSV抽出結果</h1>
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
		return nil, fmt.Errorf("パス %s の情報を取得できませんでした: %w", root, err)
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
			return nil, fmt.Errorf("ディレクトリ %s の探索中にエラーが発生しました: %w", root, err)
		}
	} else {
		entries, err := os.ReadDir(root)
		if err != nil {
			return nil, fmt.Errorf("ディレクトリ %s の読み込み中にエラーが発生しました: %w", root, err)
		}
		for _, entry := range entries {
			if err := walkFunc(filepath.Join(root, entry.Name()), entry, nil); err != nil {
				log.Printf("警告: エントリ %s を処理できませんでした: %v", entry.Name(), err)
			}
		}
	}
	return files, nil
}

// parseFlags はコマンドライン引数を解析し、設定を構成します。
func parseFlags() Config {
	var cfg Config
	var columnsStr string
	flag.StringVar(&cfg.InputPath, "in", "", "CSVファイルまたはディレクトリのパス。")
	flag.StringVar(&columnsStr, "cols", "", "抽出する列名をカンマ区切りで指定します。*で囲むと強調表示されます (例: \"氏名,*メール*\")。")
	flag.StringVar(&cfg.SearchTarget, "target", "", "行をフィルタリングするための文字列。")
	flag.StringVar(&cfg.OutFile, "out", "", "出力HTMLファイルのパス (例: results.html)。")
	flag.StringVar(&cfg.FontName, "font", "", "HTMLの値の部分に適用するフォント名 (オプション)。")
	flag.BoolVar(&cfg.Recursive, "r", false, "サブディレクトリを再帰的に検索します。")
	flag.BoolVar(&cfg.AfterOpen, "after-open", false, "処理後に出力ファイルを開きます (-outが必須)。")

	flag.Usage = func() {
		fmt.Fprintf(os.Stderr, "使用法: %s -in <パス> -cols <列1,*列2*,...> -out <ファイル.html> [オプション]\n", os.Args[0])
		fmt.Fprintln(os.Stderr, "\n使用例: go run main.go -in data -cols \"氏名,*メール*\" -target 東京 -out results.html")
		fmt.Fprintln(os.Stderr, "\nオプション:")
		flag.PrintDefaults()
	}

	flag.Parse()

	if cfg.InputPath == "" || columnsStr == "" {
		flag.Usage()
		os.Exit(1)
	}

	// アスタリスクで囲まれた列名を解析し、ColumnSpecスライスを作成
	var specs []ColumnSpec
	columns := strings.Split(columnsStr, ",")
	for _, col := range columns {
		trimmed := strings.TrimSpace(col)
		if strings.HasPrefix(trimmed, "*") && strings.HasSuffix(trimmed, "*") && len(trimmed) > 1 {
			specs = append(specs, ColumnSpec{
				Name:      trimmed[1 : len(trimmed)-1], // アスタリスクを削除
				Emphasize: true,
			})
		} else {
			specs = append(specs, ColumnSpec{
				Name:      trimmed,
				Emphasize: false,
			})
		}
	}
	cfg.Columns = specs

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
			log.Fatalf("エラー: 出力ファイル %s を作成できませんでした: %v", cfg.OutFile, err)
		}
		outputWriter = outFile
		writeHtmlHeader(outputWriter, cfg.FontName)
	} else {
		log.Println("警告: HTMLをコンソールに出力します。-outフラグで .html ファイルに保存することをお勧めします。")
	}

	files, err := findCsvFiles(cfg.InputPath, cfg.Recursive)
	if err != nil {
		log.Fatalf("エラー: %v", err)
	}

	if len(files) == 0 {
		log.Println("CSVファイルが見つかりませんでした。")
		return
	}

	for _, file := range files {
		if err := processFile(file, cfg, outputWriter); err != nil {
			log.Printf("%s の処理中にエラーが発生しました: %v", file, err)
		}
	}

	if outFile != nil {
		writeHtmlFooter(outputWriter)
		outFile.Close()
	}

	if cfg.AfterOpen && cfg.OutFile != "" {
		absPath, err := filepath.Abs(cfg.OutFile)
		if err != nil {
			log.Printf("エラー: %s の絶対パスを解決できませんでした: %v", cfg.OutFile, err)
			return
		}

		fmt.Fprintf(os.Stderr, "処理が完了しました。%s を開いています...\n", absPath)
		if err := openFile(absPath); err != nil {
			log.Printf("エラー: 出力ファイル %s を開けませんでした: %v", absPath, err)
		}
	}
}
