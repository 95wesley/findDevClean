package main

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"flag"
	"fmt"
	"io/fs"
	"net"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"findDevClean/internal/server"
	"findDevClean/web"
)

func runServe(args []string) int {
	fset := flag.NewFlagSet("serve", flag.ContinueOnError)
	addr := fset.String("addr", "127.0.0.1:7777", "endereço de escuta (use 0.0.0.0:7777 só dentro de um container)")
	var roots multiFlag
	fset.Var(&roots, "root", "pasta a varrer (repetível; padrão: home)")
	var extraHosts multiFlag
	fset.Var(&extraHosts, "allow-host", "host:porta aceito no header Host (repetível)")
	publicPort := fset.Int("public-port", 0, "porta pela qual o navegador acessa (ex.: porta publicada no Docker); padrão: a porta de escuta")
	qdirFlag := fset.String("quarantine", "", "pasta da quarentena")
	openFlag := fset.Bool("open", false, "abre a interface no navegador padrão")
	if err := fset.Parse(args); err != nil {
		return 2
	}

	home, qdir, rs, err := setup(*qdirFlag)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 1
	}
	if len(roots) == 0 {
		roots = multiFlag{home}
	}

	token, err := newToken()
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 1
	}

	ln, err := net.Listen("tcp", *addr)
	if err != nil {
		fmt.Fprintln(os.Stderr, "não foi possível escutar:", err)
		return 1
	}
	port := ln.Addr().(*net.TCPAddr).Port

	host, _, _ := net.SplitHostPort(*addr)
	if !isLoopback(host) {
		fmt.Fprintln(os.Stderr, "AVISO: escutando fora do loopback. Qualquer máquina na rede que")
		fmt.Fprintln(os.Stderr, "         alcançar esta porta poderá tentar a API (o token ainda é exigido).")
	}

	// The browser sends the published port in Host, which differs from the
	// listening port when Docker (or a proxy) remaps it.
	shown := port
	if *publicPort != 0 {
		shown = *publicPort
	}
	allowed := []string{
		fmt.Sprintf("localhost:%d", shown),
		fmt.Sprintf("127.0.0.1:%d", shown),
	}
	if shown != port {
		allowed = append(allowed, fmt.Sprintf("localhost:%d", port), fmt.Sprintf("127.0.0.1:%d", port))
	}
	allowed = append(allowed, extraHosts...)

	ui, err := fs.Sub(web.FS, "dist")
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 1
	}

	handler := server.New(server.Config{
		Home:          home,
		Roots:         roots,
		Rules:         rs,
		QuarantineDir: qdir,
		Token:         token,
		AllowedHosts:  allowed,
		UI:            ui,
	})

	srv := &http.Server{
		Handler:           handler,
		ReadHeaderTimeout: 10 * time.Second,
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	go func() {
		<-ctx.Done()
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_ = srv.Shutdown(shutdownCtx)
	}()

	fmt.Printf("findDevClean em execução. Abra no navegador:\n  http://localhost:%d/?token=%s\n", shown, token)
	if *openFlag {
		if err := openBrowser(fmt.Sprintf("http://localhost:%d/?token=%s", shown, token)); err != nil {
			fmt.Fprintln(os.Stderr, err)
		}
	}
	fmt.Println("Ctrl+C para encerrar.")

	if err := srv.Serve(ln); err != nil && err != http.ErrServerClosed {
		fmt.Fprintln(os.Stderr, err)
		return 1
	}
	return 0
}

func newToken() (string, error) {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		return "", fmt.Errorf("gerar token: %w", err)
	}
	return hex.EncodeToString(b), nil
}

func isLoopback(host string) bool {
	if host == "localhost" {
		return true
	}
	ip := net.ParseIP(host)
	return ip != nil && ip.IsLoopback()
}
