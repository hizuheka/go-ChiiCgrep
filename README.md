# SYNOPSIS
  CSVファイルまたはフォルダから指定された列の値を効率良く抽出し、コンソールに色付きで出力します。

# DESCRIPTION
  このスクリプトは、入力パス（ファイルまたはフォルダ）、抽出したい列名、そしてフィルタリング文字列をコマンドライン引数として受け取ります。
  -in には、処理するファイルまたはフォルダを指定します。
  -cols には、抽出したい列名をカンマ区切りで指定します。
  -target には、フィルタリングに使用する文字列を指定します。この文字列が、行のいずれかのセルに含まれている場合のみ、その行が処理されます。
  -r フラグを指定すると、サブディレクトリ内も再帰的に検索します。
  -no-color フラグを指定すると、色付き出力を無効にします。
  ファイルは指定された順序で一つずつ処理されます。

# EXAMPLE
```
  // 単一ファイルを処理
  go-ChiiCgrep.exe -in "data/sample.csv" -cols "ID,Name,Email"

  // ディレクトリ内のファイルを再帰的に検索し、フィルタリングして出力
  go-ChiiCgrep.exe -in "data/" -cols "ID,Name" -target "東京都" -r

  // 色を付けずに出力
  go-ChiiCgrep.exe -in "data/sample.csv" -cols "Name" -no-color
```

# NOTES
  作成者: Gemini
  最終更新日: 2024-05-20 (Improved by Gemini)
