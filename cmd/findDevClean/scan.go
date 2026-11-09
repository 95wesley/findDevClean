package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"os"

	"findDevClean/internal/rules"
	"findDevClean/internal/scanner"
)

func runScan(args []string) int {
	fs := flag.NewFlagSet("scan", flag.ContinueOnError)
	var roots multiFlag
	fs.Var(&roots, "root", "pasta a varrer (repetível; padrão: home)")
	depth := fs.Int("depth", scanner.DefaultMaxDepth, "profundidade máxima")
	asJSON := fs.Bool("json", false, "saída em JSON")
	if err := fs.Parse(args); err != nil {
		return 2
	}

	home, err := homeDir()
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 1
	}
	if len(roots) == 0 {
		roots = multiFlag{home}
	}
	rs, err := rules.Default()
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 1
	}

	res := scanner.Scan(scanner.Options{
		Roots:    roots,
		Rules:    rs,
		Home:     home,
		MaxDepth: *depth,
	})

	if *asJSON {
		enc := json.NewEncoder(os.Stdout)
		enc.SetIndent("", "  ")
		if err := enc.Encode(res); err != nil {
			fmt.Fprintln(os.Stderr, err)
			return 1
		}
		return 0
	}

	printScan(res)
	return 0
}

func printScan(res scanner.Result) {
	if len(res.Findings) == 0 {
		fmt.Println("Nada encontrado.")
	}
	for _, f := range res.Findings {
		fmt.Printf("%10s  %-11s  %-10s  %s\n",
			humanSize(f.Size), f.Risk, f.Category, shorten(f.Path))
	}
	fmt.Printf("\nTotal recuperável: %s em %d itens\n", humanSize(res.Total), len(res.Findings))
	if n := len(res.Errors); n > 0 {
		fmt.Fprintf(os.Stderr, "%d caminhos ignorados por erro (permissão, arquivos removidos)\n", n)
	}
}
