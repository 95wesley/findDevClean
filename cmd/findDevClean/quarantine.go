package main

import (
	"flag"
	"fmt"
	"os"
	"path/filepath"

	"findDevClean/internal/cleaner"
	"findDevClean/internal/rules"
)

// setup resolves the home directory, quarantine path and rules shared by the
// quarantine commands.
func setup(qdirFlag string) (home, qdir string, rs []rules.Rule, err error) {
	home, err = homeDir()
	if err != nil {
		return
	}
	qdir = qdirFlag
	if qdir == "" {
		qdir = defaultQuarantine(home)
	}
	rs, err = rules.Default()
	return
}

func runClean(args []string) int {
	fs := flag.NewFlagSet("clean", flag.ContinueOnError)
	apply := fs.Bool("apply", false, "mover de fato para a quarentena (padrão: simular)")
	qdirFlag := fs.String("quarantine", "", "pasta da quarentena")
	if err := fs.Parse(args); err != nil {
		return 2
	}
	if fs.NArg() == 0 {
		fmt.Fprintln(os.Stderr, "informe ao menos um caminho. Ex.: findDevClean clean ~/dev/app/node_modules")
		return 2
	}

	home, qdir, rs, err := setup(*qdirFlag)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 1
	}

	paths := make([]string, 0, fs.NArg())
	for _, p := range fs.Args() {
		abs, err := filepath.Abs(p)
		if err != nil {
			fmt.Fprintln(os.Stderr, err)
			return 1
		}
		paths = append(paths, abs)
	}

	rep, err := cleaner.Clean(paths, cleaner.Options{
		Home:          home,
		Rules:         rs,
		QuarantineDir: qdir,
		DryRun:        !*apply,
	})
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 1
	}

	for _, o := range rep.Outcomes {
		label := map[cleaner.Status]string{
			cleaner.StatusWouldRemove: "simular",
			cleaner.StatusRemoved:     "quarentena",
			cleaner.StatusRejected:    "recusado",
			cleaner.StatusFailed:      "falhou",
		}[o.Status]
		line := fmt.Sprintf("%-11s %10s  %s", label, humanSize(o.Size), shorten(o.Path))
		if o.Reason != "" {
			line += "  (" + o.Reason + ")"
		}
		fmt.Println(line)
	}

	if rep.DryRun {
		fmt.Printf("\nSimulação: liberaria %s. Use --apply para mover de fato.\n", humanSize(rep.Freed))
	} else {
		fmt.Printf("\nLiberou %s.\n", humanSize(rep.Freed))
		if rep.BatchID != "" {
			fmt.Printf("Para desfazer: findDevClean undo %s\n", rep.BatchID)
		}
	}
	return 0
}

func runBatches(args []string) int {
	fs := flag.NewFlagSet("batches", flag.ContinueOnError)
	qdirFlag := fs.String("quarantine", "", "pasta da quarentena")
	if err := fs.Parse(args); err != nil {
		return 2
	}
	_, qdir, _, err := setup(*qdirFlag)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 1
	}
	batches, err := cleaner.ListBatches(qdir)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 1
	}
	if len(batches) == 0 {
		fmt.Println("Quarentena vazia.")
		return 0
	}
	for _, b := range batches {
		var size int64
		for _, e := range b.Entries {
			size += e.Size
		}
		fmt.Printf("%s  %s  %d item(ns)  %s\n",
			b.ID, b.CreatedAt.Local().Format("2006-01-02 15:04"), len(b.Entries), humanSize(size))
	}
	return 0
}

func runUndo(args []string) int {
	fs := flag.NewFlagSet("undo", flag.ContinueOnError)
	qdirFlag := fs.String("quarantine", "", "pasta da quarentena")
	if err := fs.Parse(args); err != nil {
		return 2
	}
	if fs.NArg() != 1 {
		fmt.Fprintln(os.Stderr, "informe o ID do lote. Veja com: findDevClean batches")
		return 2
	}
	_, qdir, _, err := setup(*qdirFlag)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 1
	}
	outs, err := cleaner.Undo(qdir, fs.Arg(0))
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 1
	}
	failed := 0
	for _, o := range outs {
		if o.Status != cleaner.StatusRestored {
			failed++
			fmt.Printf("não restaurado: %s (%s)\n", shorten(o.Path), o.Reason)
			continue
		}
		fmt.Printf("restaurado: %s\n", shorten(o.Path))
	}
	if failed > 0 {
		return 1
	}
	return 0
}
