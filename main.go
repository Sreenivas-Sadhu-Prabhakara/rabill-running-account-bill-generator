// Command rabill-running-account-bill-generator prepares running-account bills
// for a construction contract.
//
// It keeps a bill of quantities and a measurement book, works out each RA bill
// as the work done to date less everything certified in the previous bill, and
// produces a printable bill with the account of work done and the memorandum of
// payments on it. The contractor submits that bill themselves: this tool sends
// nothing anywhere and makes no outbound network calls of any kind.
//
// It serves a small web interface on the machine it runs on and keeps
// everything in one JSON file beside the binary.
package main

import (
	"context"
	"embed"
	"errors"
	"flag"
	"fmt"
	"io/fs"
	"log"
	"net"
	"net/http"
	"net/url"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"syscall"
	"time"
)

// webUI is the interface, built into the binary so the tool is one file.
//
//go:embed web
var webUI embed.FS

const (
	defaultAddr = "127.0.0.1:8080"
	defaultData = "./rabill-data.json"

	// maxJSONBody and maxCSVBody cap what a request may send, so a mistaken
	// upload cannot exhaust memory.
	maxJSONBody = 512 << 10 // 512 KiB
	maxCSVBody  = 8 << 20   // 8 MiB
)

func main() {
	log.SetFlags(0)

	addr := flag.String("addr", defaultAddr, "address to serve the interface on")
	data := flag.String("data", defaultData, "file to keep the contract, measurements and bills in")
	flag.Usage = func() {
		out := flag.CommandLine.Output()
		fmt.Fprintln(out, "Running-account bill generator for civil contractors.")
		fmt.Fprintln(out, "Serves a local web interface and keeps one contract in one file.")
		fmt.Fprintln(out, "\nUsage:")
		flag.PrintDefaults()
	}
	flag.Parse()

	if err := run(*addr, *data); err != nil {
		log.Fatalf("rabill: %v", err)
	}
}

// run opens the contract, serves the interface and waits for an interrupt.
func run(addr, dataPath string) error {
	st, err := openStore(dataPath)
	if err != nil {
		return err
	}

	assets, err := fs.Sub(webUI, "web")
	if err != nil {
		return fmt.Errorf("reading the built-in interface: %w", err)
	}

	mux := http.NewServeMux()
	(&api{store: st}).routes(mux)
	mux.Handle("GET /", http.FileServerFS(assets))

	listener, err := net.Listen("tcp", addr)
	if err != nil {
		return fmt.Errorf("cannot serve on %s: %w", addr, err)
	}

	srv := &http.Server{
		Handler:           guard(mux),
		ReadHeaderTimeout: 10 * time.Second,
		ReadTimeout:       time.Minute,
		WriteTimeout:      2 * time.Minute,
		IdleTimeout:       2 * time.Minute,
	}

	absData, absErr := filepath.Abs(dataPath)
	if absErr != nil {
		absData = dataPath
	}
	log.Printf("Running-account bill generator")
	log.Printf("  open      http://%s", listener.Addr())
	log.Printf("  contract  %s", absData)
	log.Printf("  stop      Ctrl-C")

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	served := make(chan error, 1)
	go func() { served <- srv.Serve(listener) }()

	select {
	case err := <-served:
		if errors.Is(err, http.ErrServerClosed) {
			return nil
		}
		return err
	case <-ctx.Done():
		log.Printf("stopping")
		shutdown, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		return srv.Shutdown(shutdown)
	}
}

// guard wraps the whole interface in the few protections a local web tool
// needs: it refuses a request a page on another site has sent to it, and tells
// the browser not to guess at content types.
func guard(h http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("X-Content-Type-Options", "nosniff")
		w.Header().Set("Referrer-Policy", "no-referrer")
		w.Header().Set("Cache-Control", "no-store")

		if origin := r.Header.Get("Origin"); origin != "" && !sameOrigin(origin, r.Host) {
			http.Error(w, "this request came from another site and was refused", http.StatusForbidden)
			return
		}
		h.ServeHTTP(w, r)
	})
}

// sameOrigin reports whether an Origin header belongs to the host this server
// was reached on.
func sameOrigin(origin, host string) bool {
	u, err := url.Parse(origin)
	if err != nil || u.Host == "" {
		return false
	}
	return strings.EqualFold(u.Host, host)
}
