package main

import (
	"bufio"
	"encoding/csv"
	"flag"
	"fmt"
	"io"
	"log"
	"os"
	"os/exec"
	"path/filepath"

	// "runtime" // OS判定が不要になったため削除
	"strings"

	"github.com/fatih/color"
)

// Config はアプリケーションの設定を保持します。
type Config struct {
	InputPath    string
	Columns      []string
	SearchTarget string
	Recursive    bool
	NoColor      bool
	OutFile      string
	AfterOpen    bool
}

var (
	headerColor = color.New(color.FgCyan).SprintFunc()
	valueColor  = color.New(color.FgGreen).SprintFunc()
)

// processFile は単一のCSVファイルを処理し、指定されたwriterに出力します。
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
			if pErr, ok := err.(*csv.ParseError); ok {
				return fmt.Errorf("parse error at line %d, column %d: %w", pErr.Line, pErr.Column, pErr.Err)
			}
			return fmt.Errorf("failed to read record at line %d: %w", lineNum, err)
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
		fmt.Fprintf(&sb, "--- File: %s, Line: %d ---\n", filePath, lineNum)
		for i, colName := range targetColumns {
			idx := targetIndices[i]
			if idx < len(record) {
				fmt.Fprintf(&sb, "%s:[%s]\n", headerColor(colName), valueColor(record[idx]))
			}
		}
		if _, err := fmt.Fprint(writer, sb.String()); err != nil {
			return fmt.Errorf("failed to write to output: %w", err)
		}
	}
	return nil
}

// findCsvFiles は指定されたパスからCSVファイルのリストを検索します。
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
	flag.BoolVar(&cfg.Recursive, "r", false, "Search for CSV files recursively in subdirectories.")
	flag.BoolVar(&cfg.NoColor, "no-color", false, "Disable color output.")
	flag.StringVar(&cfg.OutFile, "out", "", "Path to the output file (optional).")
	flag.BoolVar(&cfg.AfterOpen, "after-open", false, "Open the output file after processing (requires -out).")

	flag.Usage = func() {
		fmt.Fprintf(os.Stderr, "Usage: %s -in <path> -cols <col1,col2> [options]\n", os.Args[0])
		fmt.Fprintln(os.Stderr, "Options:")
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
	// Windowsの `start` コマンドを実行する
	// `start` はパスにスペースが含まれていても正しく動作するため、ここでは単純に渡す
	cmd := exec.Command("cmd", "/c", "start", "", path)
	return cmd.Run()
}

func main() {
	log.SetFlags(0)

	cfg := parseFlags()

	var outputWriter io.Writer = os.Stdout
	var outFile *os.File // ファイルハンドルを保持する変数を宣言
	var err error

	// -out が指定されている場合はファイルを作成
	if cfg.OutFile != "" {
		// ここでは defer で閉じない
		outFile, err = os.Create(cfg.OutFile)
		if err != nil {
			log.Fatalf("Error: could not create output file %s: %v", cfg.OutFile, err)
		}
		outputWriter = outFile
	}

	if cfg.NoColor || cfg.OutFile != "" {
		color.NoColor = true
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

	// ★対策2: ファイルへの書き込みが完了した時点で、ファイルを明示的に閉じる
	if outFile != nil {
		outFile.Close()
	}

	// ★対策1: ファイルを開く前に、パスを絶対パスに変換する
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
