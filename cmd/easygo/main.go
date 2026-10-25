// Command easygo serves the Go on Easy Mode web app.
//
//	easygo                run the server
//	easygo fetch-joseki   download Kogo's Joseki Dictionary into the data dir
package main

import (
	"context"
	"encoding/hex"
	"errors"
	"flag"
	"fmt"
	"log"
	"net"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"time"

	"easygo/internal/auth"
	"easygo/internal/joseki"
	"easygo/internal/katago"
	"easygo/internal/server"
)

func main() {
	if len(os.Args) > 1 && os.Args[1] == "fetch-joseki" {
		if err := fetchJoseki(os.Args[2:]); err != nil {
			log.Fatal(err)
		}
		return
	}
	if err := run(os.Args[1:]); err != nil {
		log.Fatal(err)
	}
}

func fetchJoseki(args []string) error {
	fs := flag.NewFlagSet("fetch-joseki", flag.ExitOnError)
	data := fs.String("data", "data", "data directory")
	fs.Parse(args)
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Minute)
	defer cancel()
	path, err := joseki.FetchKogo(ctx, filepath.Join(*data, "cache"))
	if err != nil {
		return err
	}
	fmt.Println("saved", path)
	fmt.Println("Kogo's Joseki Dictionary is copyright Gary Odom; it is downloaded for your own use and is not redistributed with this app.")
	return nil
}

func env(key, def string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return def
}

func run(args []string) error {
	fs := flag.NewFlagSet("easygo", flag.ExitOnError)
	addr := fs.String("addr", env("EASYGO_ADDR", "127.0.0.1:8080"), "listen address")
	data := fs.String("data", env("EASYGO_DATA", "data"), "data directory")
	web := fs.String("web", env("EASYGO_WEB", "web/dist"), "built frontend directory ('' for API only)")
	baseURL := fs.String("base-url", env("EASYGO_BASE_URL", ""), "public URL, used for Google sign-in redirects")
	kdir := fs.String("katago-dir", env("EASYGO_KATAGO_DIR", defaultKataGoDir()), "folder holding bin/katago.exe, analysis.cfg and nets/")
	noKG := fs.Bool("no-katago", false, "do not start KataGo")
	fs.Parse(args)

	host, port, err := net.SplitHostPort(*addr)
	if err != nil {
		return fmt.Errorf("bad -addr: %w", err)
	}
	loopback := host == "127.0.0.1" || host == "localhost" || host == "::1"
	if *baseURL == "" {
		h := host
		if h == "" || h == "0.0.0.0" {
			h = "localhost"
		}
		*baseURL = "http://" + net.JoinHostPort(h, port)
	}

	if err := os.MkdirAll(*data, 0o755); err != nil {
		return err
	}
	secret, err := loadSecret(filepath.Join(*data, "secret.key"))
	if err != nil {
		return err
	}

	googleID, googleSecret := os.Getenv("GOOGLE_CLIENT_ID"), os.Getenv("GOOGLE_CLIENT_SECRET")
	// Name-only sign-in is for local use. It is on only when Google is not
	// configured and the server is bound to this machine, or when explicitly forced.
	dev := (googleID == "" && loopback) || os.Getenv("EASYGO_DEV_LOGIN") == "1"
	var admins []string
	for _, a := range strings.Split(os.Getenv("EASYGO_ADMINS"), ",") {
		if a = strings.TrimSpace(a); a != "" {
			admins = append(admins, a)
		}
	}
	if dev {
		log.Printf("dev login is ON (anyone who can reach this server can sign in with any name)")
	}

	kcfg := katagoConfig(*kdir)
	if v := os.Getenv("EASYGO_KATAGO_BIN"); v != "" {
		kcfg.Binary = v
	}
	if v := os.Getenv("EASYGO_KATAGO_MODEL"); v != "" {
		kcfg.Model = v
	}
	if v := os.Getenv("EASYGO_KATAGO_CONFIG"); v != "" {
		kcfg.Config = v
	}

	srv, err := server.New(server.Config{
		DataDir: *data, WebDir: *web, KataGo: kcfg,
		Auth: auth.Config{Secret: secret, BaseURL: *baseURL, GoogleID: googleID, GoogleSecret: googleSecret, DevLogin: dev, Admins: admins},
	})
	if err != nil {
		return err
	}
	defer srv.Close()
	srv.LoadJoseki()
	if !*noKG {
		srv.StartKataGo()
	}

	httpSrv := &http.Server{Addr: *addr, Handler: srv.Handler(), ReadHeaderTimeout: 10 * time.Second}
	stop := make(chan os.Signal, 1)
	signal.Notify(stop, os.Interrupt)
	go func() {
		<-stop
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		httpSrv.Shutdown(ctx)
	}()
	log.Printf("Go on Easy Mode listening on %s", *baseURL)
	if err := httpSrv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
		return err
	}
	return nil
}

func defaultKataGoDir() string {
	home, err := os.UserHomeDir()
	if err != nil {
		return "katago"
	}
	return filepath.Join(home, "katago")
}

// katagoConfig finds the binary, config and the first network under dir.
func katagoConfig(dir string) katago.Config {
	exe := "katago"
	if os.PathSeparator == '\\' {
		exe = "katago.exe"
	}
	cfg := katago.Config{
		Binary: filepath.Join(dir, "bin", exe),
		Config: filepath.Join(dir, "analysis.cfg"),
	}
	if nets, _ := filepath.Glob(filepath.Join(dir, "nets", "*.bin.gz")); len(nets) > 0 {
		cfg.Model = nets[0]
	}
	return cfg
}

// loadSecret reads the cookie-signing key, creating it on first run. A key
// in EASYGO_SECRET (hex) wins.
func loadSecret(path string) ([]byte, error) {
	if v := strings.TrimSpace(os.Getenv("EASYGO_SECRET")); v != "" {
		b, err := hex.DecodeString(v)
		if err != nil || len(b) < 16 {
			return nil, errors.New("EASYGO_SECRET must be at least 16 bytes of hex")
		}
		return b, nil
	}
	if b, err := os.ReadFile(path); err == nil {
		if d, err := hex.DecodeString(strings.TrimSpace(string(b))); err == nil && len(d) >= 16 {
			return d, nil
		}
	}
	b := auth.NewSecret()
	if err := os.WriteFile(path, []byte(hex.EncodeToString(b)), 0o600); err != nil {
		return nil, err
	}
	return b, nil
}
