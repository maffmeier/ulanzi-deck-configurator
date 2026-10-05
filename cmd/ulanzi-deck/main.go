package main

import (
	"context"
	_ "embed"
	"errors"
	"flag"
	"fmt"
	"io"
	"log/slog"
	"net"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"strconv"
	"syscall"
	"time"

	"github.com/maffmeier/ulanzi-deck-configurator/internal/application"
	"github.com/maffmeier/ulanzi-deck-configurator/internal/domain/deck"
	"github.com/maffmeier/ulanzi-deck-configurator/internal/infrastructure/actions"
	"github.com/maffmeier/ulanzi-deck-configurator/internal/infrastructure/autostart"
	"github.com/maffmeier/ulanzi-deck-configurator/internal/infrastructure/catalog"
	"github.com/maffmeier/ulanzi-deck-configurator/internal/infrastructure/configfile"
	"github.com/maffmeier/ulanzi-deck-configurator/internal/infrastructure/d200"
	"github.com/maffmeier/ulanzi-deck-configurator/internal/infrastructure/metrics"
	"github.com/maffmeier/ulanzi-deck-configurator/internal/infrastructure/render"
	"github.com/maffmeier/ulanzi-deck-configurator/internal/interfaces/tray"
	"github.com/maffmeier/ulanzi-deck-configurator/internal/interfaces/web"
)

var version = "dev"

//go:embed starter.yaml
var starterConfig []byte

const usage = `ulanzi-deck – Steuerung für den Ulanzi Stream Controller D200

Aufruf:
  ulanzi-deck [run] [Optionen]     Deck steuern, Editor und Tray starten (Standard)
  ulanzi-deck devices              Angeschlossene Decks auflisten
  ulanzi-deck listen               Tastendrücke anzeigen
  ulanzi-deck brightness <0-100>   Helligkeit setzen
  ulanzi-deck push [--config P]    Layout einmalig hochladen
  ulanzi-deck autostart on|off|status
  ulanzi-deck version

Optionen für run:
`

func main() {
	attachConsole()

	args := os.Args[1:]
	command := "run"
	if len(args) > 0 && args[0] != "" && args[0][0] != '-' {
		command, args = args[0], args[1:]
	}

	var err error
	switch command {
	case "run":
		err = runCommand(args)
	case "devices":
		err = devicesCommand()
	case "listen":
		err = listenCommand()
	case "brightness":
		err = brightnessCommand(args)
	case "push":
		err = pushCommand(args)
	case "autostart":
		err = autostartCommand(args)
	case "version":
		fmt.Println("ulanzi-deck", version)
	case "help", "-h", "--help":
		fmt.Print(usage)
		runFlags(nil).PrintDefaults()
	default:
		err = fmt.Errorf("unbekannter Befehl %q (siehe ulanzi-deck help)", command)
	}
	if err != nil {
		fmt.Fprintln(os.Stderr, "Fehler:", err)
		os.Exit(1)
	}
}

type runOptions struct {
	config  string
	addr    string
	open    bool
	noTray  bool
	verbose bool
}

func defaultConfigPath() string {
	dir, err := os.UserConfigDir()
	if err != nil {
		return "deck.yaml"
	}
	return filepath.Join(dir, "ulanzi-deck", "deck.yaml")
}

func runFlags(opts *runOptions) *flag.FlagSet {
	if opts == nil {
		opts = &runOptions{}
	}
	fs := flag.NewFlagSet("run", flag.ContinueOnError)
	fs.StringVar(&opts.config, "config", defaultConfigPath(), "Pfad zur deck.yaml")
	fs.StringVar(&opts.addr, "addr", "127.0.0.1:8765", "Adresse des Editors")
	fs.BoolVar(&opts.open, "open", false, "Editor beim Start im Browser öffnen")
	fs.BoolVar(&opts.noTray, "no-tray", false, "kein Tray-Icon anzeigen")
	fs.BoolVar(&opts.verbose, "v", false, "ausführliche Logs")
	return fs
}

func newLogger(verbose bool, logFile io.Writer) *slog.Logger {
	level := slog.LevelInfo
	if verbose {
		level = slog.LevelDebug
	}
	var w io.Writer = os.Stderr
	if logFile != nil {
		w = io.MultiWriter(os.Stderr, logFile)
	}
	return slog.New(slog.NewTextHandler(w, &slog.HandlerOptions{Level: level}))
}

// openLogFile keeps a log next to the user cache; GUI builds on Windows and
// autostarted processes have no visible stderr.
func openLogFile() *os.File {
	dir, err := os.UserCacheDir()
	if err != nil {
		return nil
	}
	dir = filepath.Join(dir, "ulanzi-deck")
	if os.MkdirAll(dir, 0o755) != nil {
		return nil
	}
	path := filepath.Join(dir, "ulanzi-deck.log")
	flags := os.O_CREATE | os.O_WRONLY | os.O_APPEND
	if info, err := os.Stat(path); err == nil && info.Size() > 5<<20 {
		flags |= os.O_TRUNC
	}
	f, err := os.OpenFile(path, flags, 0o644)
	if err != nil {
		return nil
	}
	return f
}

func ensureConfig(path string, log *slog.Logger) (created bool) {
	if _, err := os.Stat(path); err == nil {
		return false
	}
	if err := configfile.WriteAtomic(path, starterConfig); err != nil {
		log.Error("cannot create starter config", "path", path, "error", err)
		return false
	}
	log.Info("starter config created", "path", path)
	return true
}

func fallbackConfig() *deck.Config {
	return &deck.Config{
		Pages:       []deck.Page{{Name: deck.EditorDefaultPage}},
		DefaultPage: deck.EditorDefaultPage,
		SmallWindow: deck.DefaultSmallWindow(),
		Brightness:  deck.DefaultBrightness,
	}
}

func runCommand(args []string) error {
	var opts runOptions
	fs := runFlags(&opts)
	if err := fs.Parse(args); err != nil {
		return err
	}
	configPath, err := filepath.Abs(configfile.ExpandHome(opts.config))
	if err != nil {
		return err
	}

	logFile := openLogFile()
	if logFile != nil {
		defer logFile.Close()
	}
	log := newLogger(opts.verbose, logFile)
	slog.SetDefault(log)

	editorURL := "http://" + opts.addr + "/"
	runner := actions.NewRunner(log)
	openEditor := func() {
		if err := runner.Run(deck.Action{Type: deck.ActionURL, URL: editorURL}); err != nil {
			log.Error("cannot open browser", "error", err)
		}
	}

	ln, err := net.Listen("tcp", opts.addr)
	if err != nil {
		// Most likely another instance owns the port: just bring it up.
		log.Info("editor address in use, opening running instance", "addr", opts.addr, "error", err)
		openEditor()
		return nil
	}

	created := ensureConfig(configPath, log)
	cfg, err := configfile.Load(configPath)
	if err != nil {
		log.Error("config invalid, starting with an empty layout until it is fixed", "path", configPath, "error", err)
		cfg = fallbackConfig()
	}

	cat, err := catalog.Load()
	if err != nil {
		return fmt.Errorf("icon catalog: %w", err)
	}
	reader := metrics.NewReader()

	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer cancel()

	dev := d200.NewDevice(d200.OpenHID, log.With("component", "deck"))
	engine := application.NewWidgetEngine(reader, widgetSources{runner: runner}, render.Renderer{}, log.With("component", "widgets"))
	daemon := application.NewDaemon(dev, runner, reader, render.Renderer{}, engine, cfg, log.With("component", "daemon"))

	go dev.Run(ctx)
	go daemon.Run(ctx)
	go application.WatchConfig(ctx, configPath, configfile.Load, daemon.ApplyConfig, log.With("component", "watcher"))

	server := &web.Server{
		ConfigPath: configPath,
		Version:    version,
		Catalog:    cat,
		Metrics:    reader,
		Connected:  dev.Connected,
		Widgets:    engine,
		Log:        log.With("component", "web"),
	}
	httpServer := &http.Server{Handler: server.Handler(), ReadHeaderTimeout: 10 * time.Second}
	go func() {
		if err := httpServer.Serve(ln); err != nil && !errors.Is(err, http.ErrServerClosed) {
			log.Error("editor server stopped", "error", err)
			cancel()
		}
	}()
	log.Info("ulanzi-deck running", "version", version, "editor", editorURL, "config", configPath)

	if opts.open || created {
		openEditor()
	}

	exe, _ := os.Executable()
	if tray.Available && !opts.noTray {
		go func() {
			<-ctx.Done()
			tray.Stop()
		}()
		tray.Run(tray.Menu{
			OpenEditor:      openEditor,
			AutostartActive: autostart.Enabled,
			ToggleAutostart: func() error {
				if autostart.Enabled() {
					return autostart.Disable()
				}
				return autostart.Enable(exe, []string{"--config", configPath})
			},
			Quit: cancel,
		})
	} else {
		<-ctx.Done()
	}

	log.Info("shutting down")
	shutdownCtx, done := context.WithTimeout(context.Background(), 2*time.Second)
	defer done()
	_ = httpServer.Shutdown(shutdownCtx)
	// Let the device loop close the HID handle.
	time.Sleep(200 * time.Millisecond)
	return nil
}

func devicesCommand() error {
	devices, err := d200.Enumerate()
	if err != nil {
		return err
	}
	if len(devices) == 0 {
		fmt.Println("Kein Ulanzi D200 gefunden.")
		return nil
	}
	for i, d := range devices {
		fmt.Printf("%d  %s %s  Seriennummer %s  (%s)\n", i, d.Manufacturer, d.Product, d.Serial, d.Path)
	}
	return nil
}

// withDevice opens the deck directly, runs fn and closes it again.
func withDevice(fn func(dev *d200.Device) error) error {
	log := newLogger(false, nil)
	t, err := d200.OpenHID()
	if err != nil {
		return err
	}
	opened := false
	dev := d200.NewDevice(func() (d200.Transport, error) {
		if opened {
			return nil, d200.ErrDeviceNotFound
		}
		opened = true
		return t, nil
	}, log)
	ctx, cancel := context.WithCancel(context.Background())
	go dev.Run(ctx)
	defer func() {
		cancel()
		time.Sleep(100 * time.Millisecond)
	}()
	for !dev.Connected() {
		time.Sleep(10 * time.Millisecond)
	}
	return fn(dev)
}

func brightnessCommand(args []string) error {
	if len(args) != 1 {
		return errors.New("Aufruf: ulanzi-deck brightness <0-100>")
	}
	value, err := strconv.Atoi(args[0])
	if err != nil {
		return err
	}
	return withDevice(func(dev *d200.Device) error {
		if err := dev.SetBrightness(value, true); err != nil {
			return err
		}
		fmt.Println("Helligkeit gesetzt:", value)
		return nil
	})
}

func pushCommand(args []string) error {
	var opts runOptions
	fs := runFlags(&opts)
	if err := fs.Parse(args); err != nil {
		return err
	}
	cfg, err := configfile.Load(configfile.ExpandHome(opts.config))
	if err != nil {
		return err
	}
	return withDevice(func(dev *d200.Device) error {
		var visible []deck.Button
		for _, b := range cfg.ButtonsFor(cfg.DefaultPage) {
			if b.Index < deck.ButtonCount {
				visible = append(visible, b)
			}
		}
		if err := dev.SetBrightness(cfg.Brightness, true); err != nil {
			return err
		}
		if err := dev.SetButtons(visible, false); err != nil {
			return err
		}
		fmt.Printf("Seite %q hochgeladen (%d Tasten).\n", cfg.DefaultPage, len(visible))
		return nil
	})
}

func listenCommand() error {
	return withDevice(func(dev *d200.Device) error {
		fmt.Println("Warte auf Tastendrücke (Strg+C beendet) …")
		ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
		defer cancel()
		ticker := time.NewTicker(2 * time.Second)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return nil
			case <-ticker.C:
				_ = dev.KeepAlive()
			case ev := <-dev.Events():
				switch e := ev.(type) {
				case deck.ButtonEvent:
					state := "losgelassen"
					if e.Pressed {
						state = "gedrückt"
					}
					fmt.Printf("%s  Taste %2d %s\n", e.OccurredAt.Format("15:04:05.000"), e.Index, state)
				case deck.DeviceInfoEvent:
					fmt.Println("Geräteinfo:", e.Info)
				case deck.ConnectionEvent:
					if !e.Connected {
						return fmt.Errorf("Verbindung verloren: %v", e.Err)
					}
				}
			}
		}
	})
}

func autostartCommand(args []string) error {
	if len(args) != 1 {
		return errors.New("Aufruf: ulanzi-deck autostart on|off|status")
	}
	switch args[0] {
	case "on":
		exe, err := os.Executable()
		if err != nil {
			return err
		}
		if err := autostart.Enable(exe, nil); err != nil {
			return err
		}
		fmt.Println("Autostart aktiviert.")
	case "off":
		if err := autostart.Disable(); err != nil {
			return err
		}
		fmt.Println("Autostart deaktiviert.")
	case "status":
		fmt.Println("Autostart aktiv:", autostart.Enabled())
	default:
		return errors.New("Aufruf: ulanzi-deck autostart on|off|status")
	}
	return nil
}
