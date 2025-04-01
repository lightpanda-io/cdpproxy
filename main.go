// Copyright 2025 Lightpanda (Selecy SAS)
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package main

import (
	"context"
	"flag"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"os"

	"github.com/gorilla/websocket"
)

var upgrader = websocket.Upgrader{}

const (
	exitOK   = 0
	exitFail = 1
)

// main starts interruptable context and runs the program.
func main() {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	err := run(ctx, os.Args, os.Stdout, os.Stderr)
	if err != nil {
		fmt.Fprintln(os.Stderr, err.Error())
		os.Exit(exitFail)
	}

	os.Exit(exitOK)
}

const (
	CdpUrlDefault  = "ws://127.0.0.1:9223"
	AddressDefault = "127.0.0.1:9222"
)

var cdpurl = CdpUrlDefault

func run(_ context.Context, args []string, stdout, stderr io.Writer) error {
	// declare runtime flag parameters.
	flags := flag.NewFlagSet(args[0], flag.ExitOnError)
	flags.SetOutput(stderr)

	var (
		verbose = flags.Bool("verbose", false, "enable debug log level")
		addr    = flags.String("addr", env("CDP_ADDRESS", AddressDefault), "api address listen to, used only when daemon is true")
	)

	// usage func declaration.
	exec := args[0]
	flags.Usage = func() {
		fmt.Fprintf(stderr, "usage: %s [<cdp url>]\n", exec)
		fmt.Fprintf(stderr, "proxy for CDP connection.\n")
		fmt.Fprintf(stderr, "\nCommand line options:\n")
		flags.PrintDefaults()
		fmt.Fprintf(stderr, "\nEnvironment vars:\n")
		fmt.Fprintf(stderr, "\tCDP_ADDRESS\tdefault %s\n", AddressDefault)
	}
	if err := flags.Parse(args[1:]); err != nil {
		return err
	}

	if *verbose {
		slog.SetLogLoggerLevel(slog.LevelDebug)
	}

	args = flags.Args()

	if len(args) > 1 {
		return fmt.Errorf("too many arguments")
	}

	if len(args) == 1 {
		cdpurl = args[0]
	}

	fmt.Fprintf(os.Stderr, "ws server listening on ws://%s\n", *addr)
	http.HandleFunc("/", ws(cdpurl))

	return http.ListenAndServe(*addr, nil)
}

func ws(cdpurl string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		ws, err := upgrader.Upgrade(w, r, nil)
		if err != nil {
			slog.Error("ws upgrade", slog.Any("err", err))
			return
		}
		defer ws.Close()

		if err := proxy(r.Context(), cdpurl, ws); err != nil {
			slog.Error("proxy", slog.Any("err", err))
			return
		}
	}
}

func proxy(ctx context.Context, cdpurl string, ws *websocket.Conn) error {
	conn, _, err := websocket.DefaultDialer.Dial(cdpurl, nil)
	if err != nil {
		return fmt.Errorf("ws conn: %w", err)
	}
	defer conn.Close()

	ctx, cancel := context.WithCancel(ctx)
	defer cancel()

	// conn read -> WS write
	go func() {
		defer cancel()

		for {
			if ctx.Err() != nil {
				return
			}

			mt, msg, err := conn.ReadMessage()
			if err != nil {
				slog.Error("conn read", slog.Any("err", err))
				return
			}
			fmt.Println("< " + string(msg))

			if err = ws.WriteMessage(mt, msg); err != nil {
				slog.Error("ws write", slog.Any("err", err))
				return
			}
		}
	}()

	// WS read -> conn write
	go func() {
		defer cancel()

		for {
			if ctx.Err() != nil {
				return
			}

			mt, msg, err := ws.ReadMessage()
			if err != nil {
				slog.Error("ws read", slog.Any("err", err))
				return
			}
			fmt.Println("> " + string(msg))
			if err = conn.WriteMessage(mt, msg); err != nil {
				slog.Error("conn write", slog.Any("err", err))
				return
			}
		}
	}()

	select {
	case <-ctx.Done():
		return nil
	}
}

// env returns the env value corresponding to the key or the default string.
func env(key, dflt string) string {
	val, ok := os.LookupEnv(key)
	if !ok {
		return dflt
	}

	return val
}
