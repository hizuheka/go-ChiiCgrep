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

// 複数の -highlight-if 引数を保持するためのカスタム型
type highlightConditions []string

// flag.Valueインターフェースを実装するためのString()メソッド
func (h *highlightConditions) String() string {
	return strings.Join(*h, ", ")
}

// flag.Valueインターフェースを実装するためのSet()メソッド
// -highlight-if が指定されるたびにこのメソッドが呼ばれる
func (h *highlightConditions) Set(value string) error {
	*h = append(*h, value)
	return nil
}

// 複数の -tag-file 引数を保持するためのカスタム型
type fileTagConditions []string

func (f *fileTagConditions) String() string { return strings.Join(*f, ", ") }
func (f *fileTagConditions) Set(value string) error {
	*f = append(*f, value)
	return nil
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
	HighlightIfs highlightConditions // stringからカスタム型へ
	FileTags     fileTagConditions   // 追加
}

// ハイライト条件を構造化して保持するための型
type highlightRule struct {
	ColumnName  string
	ColumnValue string
}

// ファイルタグ条件を構造化して保持するための型
type fileTagRule struct {
	TagName string
	Keyword string
}

// processFile は単一のCSVファイルを処理し、HTML形式でwriterに出力します。
func processFile(filePath string, cfg Config, writer io.Writer, rules []highlightRule, tagRules []fileTagRule) error {
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

	type resolvedRule struct {
		Index int
		Value string
	}
	var resolvedRules []resolvedRule
	for _, rule := range rules {
		if idx, ok := headerMap[rule.ColumnName]; ok {
			resolvedRules = append(resolvedRules, resolvedRule{Index: idx, Value: rule.ColumnValue})
		} else {
			log.Printf("警告: 行ハイライト条件の列 '%s' がファイル %s に見つかりません。", rule.ColumnName, filePath)
		}
	}

	type targetColumn struct {
		Name      string
		Index     int
		Emphasize bool
	}
	var targetColumns []targetColumn
	for _, spec := range cfg.Columns {
		if idx, ok := headerMap[spec.Name]; ok {
			targetColumns = append(targetColumns, targetColumn{
				Name: spec.Name, Index: idx, Emphasize: spec.Emphasize,
			})
		} else {
			log.Printf("警告: 列 '%s' がファイル %s に見つかりません", spec.Name, filePath)
		}
	}

	if len(targetColumns) == 0 {
		log.Printf("警告: 指定された列が %s に見つかりませんでした。このファイルをスキップします。", filePath)
		return nil
	}

	// ファイル名に適用するタグを決定
	fileTagClass := ""
	for _, tagRule := range tagRules {
		if strings.Contains(filePath, tagRule.Keyword) {
			fileTagClass = " tag-" + html.EscapeString(tagRule.TagName)
			break // 最初に見つかったタグを適用
		}
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

		// この行でハイライトすべき「列のインデックス」をマップに記録する
		columnsToHighlight := make(map[int]bool)
		for _, rule := range resolvedRules {
			if rule.Index < len(record) && record[rule.Index] == rule.Value {
				// 条件が一致した場合、その条件の列インデックスをハイライト対象としてマーク
				columnsToHighlight[rule.Index] = true
			}
		}

		var sb strings.Builder
		// 行全体のハイライトは行わないため、クラス指定を削除
		fmt.Fprintln(&sb, "<div class=\"record\">")
		// file-info に決定したタグクラスを追加
		fmt.Fprintf(&sb, "  <p class=\"file-info%s\">--- ファイル: %s, 行: %d ---</p>\n", fileTagClass, html.EscapeString(filePath), lineNum)

		for _, col := range targetColumns {
			idx := col.Index
			if idx < len(record) {
				key := html.EscapeString(col.Name)
				value := html.EscapeString(record[idx])
				className := "data-item"
				if col.Emphasize {
					className += " emphasis"
				}
				// この列がハイライト対象かをマップでチェックし、クラスを追加
				if columnsToHighlight[col.Index] {
					className += " highlight-value"
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
    body { font-family: -apple-system, BlinkMacSystemFont, "Segoe UI", Roboto, sans-serif; background-color: #f4f4f9; color: #333; margin: 0; padding: 20px; }
    h1 { font-size: 1.5em; margin-top: 0; margin-bottom: 12px; padding-bottom: 8px; border-bottom: 1px solid #ccc; }
    .record { background-color: #fff; border: 1px solid #ddd; border-radius: 8px; padding: 15px; margin-bottom: 15px; box-shadow: 0 2px 4px rgba(0,0,0,0.1); }
    .data-item { margin-top: 0; margin-bottom: 0; padding: 2px 4px; border-radius: 3px; }
    .emphasis { font-weight: bold; background-color: #fff8c4; }
    .file-info { font-size: 0.9em; color: #666; border-bottom: 1px solid #eee; padding-bottom: 10px; margin-top: 0; margin-bottom: 8px; }
    .header { color: #007bff; font-weight: bold; }
    .value { color: #28a745; %s }
    /* .highlight-row を削除し、セルをハイライトする .highlight-value を追加 */
    .highlight-value {
      background-color: #e7f3ff; /* 薄い青色の背景 */
      border-left: 3px solid #007bff;
      margin-left: -7px; /* ボーダーとパディングを調整 */
      padding-left: 4px;
    }
	/* ファイルタグ用のスタイル */
    .tag-important { font-weight: bold; color: #721c24; background-color: #f8d7da; border-left-color: #f5c6cb; }
    .tag-warning { font-weight: bold; color: #856404; background-color: #fff3cd; border-left-color: #ffeeba; }
    .tag-archived { color: #6c757d; background-color: #e2e3e5; border-left-color: #d6d8db; font-style: italic; }
    .tag-completed { color: #155724; background-color: #d4edda; border-left-color: #c3e6cb; }

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
	flag.StringVar(&columnsStr, "cols", "", "抽出する列名をカンマ区切りで指定します。*で囲むとセルが強調されます。")
	flag.StringVar(&cfg.SearchTarget, "target", "", "行をフィルタリングするための文字列。")
	flag.StringVar(&cfg.OutFile, "out", "", "出力HTMLファイルのパス。")
	flag.StringVar(&cfg.FontName, "font", "", "値に適用するフォント名 (オプション)。")
	// flag.Var を使って複数回の指定を可能にする
	flag.Var(&cfg.HighlightIfs, "highlight-if", "行全体を強調表示する条件 (例: \"ステータス=完了\")。複数指定可能。")
	// 新しいフラグを定義
	flag.Var(&cfg.FileTags, "tag-file", "ファイル名をキーワードでタグ付けし強調表示します (例: \"important:final_report\")。\n利用可能なタグ: important, warning, completed, archived。複数指定可能。")
	flag.BoolVar(&cfg.Recursive, "r", false, "サブディレクトリを再帰的に検索します。")
	flag.BoolVar(&cfg.AfterOpen, "after-open", false, "処理後に出力ファイルを開きます (-outが必須)。")

	flag.Usage = func() {
		fmt.Fprintf(os.Stderr, "使用法: %s -in <パス> -cols <...> [オプション]\n", os.Args[0])
		fmt.Fprintln(os.Stderr, "\n使用例: go-ChiiCgrep.exe -in data -cols \"氏名\" -tag-file \"important:final\" -tag-file \"warning:error\"")
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
				Name: trimmed[1 : len(trimmed)-1], Emphasize: true,
			})
		} else {
			specs = append(specs, ColumnSpec{
				Name: trimmed, Emphasize: false,
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

	// 複数のハイライト条件を解析
	var highlightRules []highlightRule
	for _, cond := range cfg.HighlightIfs {
		parts := strings.SplitN(cond, "=", 2)
		if len(parts) == 2 {
			highlightRules = append(highlightRules, highlightRule{
				ColumnName:  strings.TrimSpace(parts[0]),
				ColumnValue: strings.TrimSpace(parts[1]),
			})
		} else {
			log.Fatalf("エラー: -highlight-if の書式が正しくありません: %s。\"列名=値\" の形式で指定してください。", cond)
		}
	}

	// ファイルタグ条件を解析
	var fileTagRules []fileTagRule
	for _, tagCond := range cfg.FileTags {
		parts := strings.SplitN(tagCond, ":", 2)
		if len(parts) == 2 && parts[0] != "" && parts[1] != "" {
			fileTagRules = append(fileTagRules, fileTagRule{
				TagName: strings.TrimSpace(parts[0]),
				Keyword: strings.TrimSpace(parts[1]),
			})
		} else {
			log.Fatalf("エラー: -tag-file の書式が正しくありません: %s。\"タグ名:キーワード\" の形式で指定してください。", tagCond)
		}
	}

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
		// 解析したハイライト条件のスライスをprocessFileに渡す
		if err := processFile(file, cfg, outputWriter, highlightRules, fileTagRules); err != nil {
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
