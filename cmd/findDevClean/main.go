// Command findDevClean finds and safely frees developer caches and build artifacts.
package main

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

const usage = `findDevClean — libera espaço de caches e artefatos de build

Uso:
  findDevClean scan    [--root DIR]... [--depth N] [--json]
  findDevClean clean   [--apply] [--quarantine DIR] CAMINHO...
  findDevClean batches [--quarantine DIR]
  findDevClean undo    [--quarantine DIR] ID_DO_LOTE
  findDevClean serve   [--addr HOST:PORTA] [--root DIR]... [--quarantine DIR]
                       [--allow-host HOST:PORTA]...

"clean" só simula, a menos que --apply seja passado. Nada é apagado de
imediato: os itens vão para a quarentena (~/.findDevClean/quarantine) e
podem ser restaurados com "undo".
`

func main() {
	// Without a command the app starts its server on a free port and opens
	// the browser, so double-clicking the binary is enough on Windows and Linux.
	if len(os.Args) < 2 {
		os.Exit(runServe([]string{"--open", "--addr", "127.0.0.1:0"}))
	}
	args := os.Args[2:]
	var code int
	switch os.Args[1] {
	case "scan":
		code = runScan(args)
	case "clean":
		code = runClean(args)
	case "batches":
		code = runBatches(args)
	case "undo":
		code = runUndo(args)
	case "serve":
		code = runServe(args)
	case "-h", "--help", "help":
		fmt.Print(usage)
		return
	default:
		fmt.Fprintf(os.Stderr, "comando desconhecido: %q\n\n%s", os.Args[1], usage)
		code = 2
	}
	os.Exit(code)
}

type multiFlag []string

func (m *multiFlag) String() string     { return fmt.Sprint([]string(*m)) }
func (m *multiFlag) Set(v string) error { *m = append(*m, v); return nil }

func homeDir() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("não foi possível descobrir a pasta home: %w", err)
	}
	return home, nil
}

func defaultQuarantine(home string) string {
	return filepath.Join(home, ".findDevClean", "quarantine")
}

func humanSize(b int64) string {
	const unit = 1024
	if b < unit {
		return fmt.Sprintf("%d B", b)
	}
	div, exp := int64(unit), 0
	for n := b / unit; n >= unit; n /= unit {
		div *= unit
		exp++
	}
	return fmt.Sprintf("%.1f %ciB", float64(b)/float64(div), "KMGTPE"[exp])
}

// shorten replaces the home directory prefix with ~ for display.
func shorten(p string) string {
	home, err := os.UserHomeDir()
	if err != nil {
		return p
	}
	rel, err := filepath.Rel(home, p)
	if err == nil && rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		return filepath.Join("~", rel)
	}
	return p
}
