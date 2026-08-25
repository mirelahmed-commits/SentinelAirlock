package cli

import (
	"fmt"
	"net"
	"os"
	"os/signal"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/spf13/cobra"
	"github.com/yourname/sentinel-airlock/internal/index"
	"github.com/yourname/sentinel-airlock/internal/web"
)

func serveCmd() *cobra.Command {
	var listen string
	var open bool
	var noOpen bool
	var readOnly bool
	var port int
	var background bool
	var status bool
	var stop bool
	var managed bool
	cmd := &cobra.Command{
		Use:   "serve",
		Short: "Start local web evidence viewer",
		Long: `Start the local web evidence viewer.

Two modes:
  Read-only inspection:  airlock serve --read-only    (safe to share; state
                         changes appear as terminal commands, never buttons)
  Operator (default):    airlock serve                (local trusted control;
                         review / rollback / export can run from the UI)

Lifecycle:
  airlock serve                      foreground operator viewer
  airlock serve --read-only          foreground read-only viewer
  airlock serve --background --open  detached viewer, returns the terminal
  airlock serve --status             show the running viewer (mode/URL/PID/log)
  airlock serve --stop               stop the running viewer

Rollback from the UI (operator mode only) restores the Airlock workspace
(.airlock/workspaces/<run_id>/repo), never your original repo.`,
		RunE: func(cmd *cobra.Command, args []string) error {
			// Lifecycle sub-actions.
			if status {
				return viewerStatus()
			}
			if stop {
				return viewerStop()
			}

			if noOpen {
				open = false
			}
			// Resolve the concrete listen address.
			if port > 0 {
				host := "127.0.0.1"
				if strings.Contains(listen, ":") {
					parts := strings.Split(listen, ":")
					host = strings.Join(parts[:len(parts)-1], ":")
				}
				listen = host + ":" + strconv.Itoa(port)
			}

			mode := viewerModeLabel(readOnly)

			// Refuse to start a duplicate viewer.
			if existing, ok := runningViewer(); ok {
				fmt.Printf("A local viewer is already running.\n")
				fmt.Printf("  Mode: %s\n  URL:  %s\n  PID:  %d\n  Log:  %s\n", existing.Mode, existing.URL, existing.PID, existing.Log)
				fmt.Printf("Stop it first with: airlock serve --stop\n")
				return nil
			}

			// Background: spawn a detached child and return the terminal.
			if background {
				return startViewerBackground(listen, readOnly, open)
			}

			// Foreground (direct, or the managed background child).
			refreshIndex()
			ln, err := net.Listen("tcp", listen)
			if err != nil {
				return fmt.Errorf("unable to bind %s (try --port 8081): %w", listen, err)
			}
			url := "http://" + ln.Addr().String()

			logDesc := "(foreground terminal)"
			if managed {
				logDesc = viewerLogPath()
			}
			_ = writeViewerMeta(viewerMeta{
				PID:        os.Getpid(),
				Mode:       mode,
				URL:        url,
				Listen:     ln.Addr().String(),
				Log:        logDesc,
				Started:    time.Now().Format(time.RFC3339),
				Background: managed,
			})
			// Remove our metadata on clean shutdown so --status stays accurate.
			installViewerCleanup()

			store, _ := index.Load(index.DefaultPath())
			fmt.Println("Viewer started")
			fmt.Printf("Mode: %s\n", mode)
			fmt.Printf("URL: %s\n", url)
			fmt.Printf("Read-only: %t\n", readOnly)
			fmt.Printf("Index: %s\n", index.DefaultPath())
			fmt.Printf("Runs indexed: %d\n", len(store.Runs))
			if !managed {
				fmt.Printf("Stop: Ctrl-C  (or run in background: airlock serve --background)\n")
			}
			shutdownFn := func() {
				removeViewerMeta()
				os.Exit(0)
			}
			return web.StartOnListener(ln, open, readOnly, shutdownFn)
		},
	}
	cmd.Flags().StringVar(&listen, "listen", "127.0.0.1:8080", "Listen address")
	cmd.Flags().BoolVar(&open, "open", false, "Open browser")
	cmd.Flags().BoolVar(&noOpen, "no-open", false, "Do not open browser")
	cmd.Flags().IntVar(&port, "port", 0, "Port override")
	cmd.Flags().BoolVar(&readOnly, "read-only", false, "Read-only inspection mode (no mutating actions)")
	cmd.Flags().BoolVar(&background, "background", false, "Run the viewer detached in the background")
	cmd.Flags().BoolVar(&status, "status", false, "Show the running viewer's status")
	cmd.Flags().BoolVar(&stop, "stop", false, "Stop the running viewer")
	// --managed marks the detached child; it manages viewer.json + cleanup.
	cmd.Flags().BoolVar(&managed, "managed", false, "Internal: run as the managed background viewer")
	_ = cmd.Flags().MarkHidden("managed")
	return cmd
}

// installViewerCleanup removes the viewer metadata when the serving process is
// asked to shut down, so status/stop never report a ghost viewer.
func installViewerCleanup() {
	ch := make(chan os.Signal, 1)
	signal.Notify(ch, syscall.SIGINT, syscall.SIGTERM)
	go func() {
		<-ch
		removeViewerMeta()
		os.Exit(0)
	}()
}
