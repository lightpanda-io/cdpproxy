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
	"crypto/md5"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"os"
	"strings"
	"time"

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

func run(_ context.Context, args []string, _, stderr io.Writer) error {
	// declare runtime flag parameters.
	flags := flag.NewFlagSet(args[0], flag.ExitOnError)
	flags.SetOutput(stderr)

	var (
		level     = flags.String("level", "info", "verbosity (debug, info, warn, error)")
		filter    = flags.Bool("filter", false, "filter output")
		nonetwork = flags.Bool("no-network", false, "filter out Network events, --filter must be set")
		nolog     = flags.Bool("no-log", false, "filter out Log events, --filter must be set")
		noid      = flags.Bool("no-id", false, "filter out message's ids, --filter must be set")
		timestamp = flags.Bool("timestamp", false, "add timestamp info to messages")
		connid    = flags.Bool("connid", false, "add client connid info to messages")
		addr      = flags.String("addr", env("CDP_ADDRESS", AddressDefault), "api address listen to, used only when daemon is true")
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

	switch *level {
	case "debug":
		slog.SetLogLoggerLevel(slog.LevelDebug)
	case "info":
		slog.SetLogLoggerLevel(slog.LevelInfo)
	case "warn":
		slog.SetLogLoggerLevel(slog.LevelWarn)
	case "error":
		slog.SetLogLoggerLevel(slog.LevelError)
	default:
		return fmt.Errorf("invalid log level")
	}

	logf := logFunc(logFuncOpt{
		Filter:    *filter,
		NoNetwork: *nonetwork,
		NoLog:     *nolog,
		NoId:      *noid,
		Timestamp: *timestamp,
		ConnId:    *connid,
	})

	args = flags.Args()

	if len(args) > 1 {
		return fmt.Errorf("too many arguments")
	}

	if len(args) == 1 {
		cdpurl = args[0]
	}

	fmt.Fprintf(os.Stderr, "ws server listening on ws://%s\n", *addr)
	http.HandleFunc("/", ws(cdpurl, logf))

	return http.ListenAndServe(*addr, nil)
}

func ws(cdpurl string, logf LogFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		r.Header.Write(os.Stderr)
		ws, err := upgrader.Upgrade(w, r, nil)
		if err != nil {
			slog.Error("ws upgrade", slog.Any("err", err))
			return
		}
		defer ws.Close()

		if err := proxy(r.Context(), cdpurl, ws, logf); err != nil {
			slog.Error("proxy", slog.Any("err", err))
			return
		}
	}
}

type LogFunc = func(source, connid string, data []byte)

type logFuncOpt = struct {
	Filter bool
	// filter Network events
	NoNetwork bool
	// filter Log events
	NoLog     bool
	NoId      bool
	Timestamp bool
	ConnId    bool
}

func logFunc(opt logFuncOpt) LogFunc {
	if opt.Filter == false {
		// no filter
		return func(source, connid string, data []byte) {
			switch source {
			case "wswrite":
				fmt.Print("< ")
			case "wsread":
				fmt.Print("> ")
			}
			if opt.ConnId {
				fmt.Printf("%s ", connid)
			}
			if opt.Timestamp {
				fmt.Printf("%v ", time.Now())
			}
			fmt.Println(string(data))
		}
	}

	return func(source, connid string, data []byte) {
		data = cleanup(opt, "root", data)
		if data == nil {
			// skip the row
			return
		}

		switch source {
		case "wswrite":
			fmt.Print("< ")
		case "wsread":
			fmt.Print("> ")
		}
		if opt.ConnId {
			fmt.Printf("%s ", connid)
		}
		if opt.Timestamp {
			fmt.Printf("%v ", time.Now())
		}
		fmt.Printf(string(data))
	}
}

func cleanup(opt logFuncOpt, k string, data []byte) []byte {
	var m map[string]json.RawMessage
	if err := json.Unmarshal(data, &m); err != nil {
		slog.Error("unmarshal json", slog.Any("err", err), slog.String("key", k))
		return nil
	}
	for k, v := range m {
		if strings.HasSuffix(k, "Id") {
			m[k] = []byte("\"\"")
			continue
		}
		switch k {
		case "method":
			if opt.NoNetwork || opt.NoLog {
				var method string
				if err := json.Unmarshal(v, &method); err != nil {
					slog.Error("unmarshal json", slog.Any("err", err), slog.String("key", k))
					return nil
				}
				if opt.NoNetwork && strings.HasPrefix(method, "Network.") {
					return nil
				}
				if opt.NoLog && strings.HasPrefix(method, "Log.") {
					return nil
				}
			}
		case "id":
			if opt.NoId {
				m[k] = []byte("\"\"")
			}
		case "params", "result", "initiator", "context", "targetInfo", "frame":
			m[k] = json.RawMessage(cleanup(opt, k, v))
		case "arguments", "request", "response", "timestamp", "stack", "headers":
			m[k] = []byte("\"\"")
		case "expression", "functionDeclaration", "value":
			m[k] = []byte(fmt.Sprintf("\"%x\"", md5.Sum([]byte(v))))
		}
	}

	raw, err := json.Marshal(m)
	if err != nil {
		slog.Error("marshal json", slog.Any("err", err))
		return nil
	}
	return raw
}

func proxy(ctx context.Context, cdpurl string, ws *websocket.Conn, logf LogFunc) error {
	connid := ws.RemoteAddr().String()

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
				slog.Error("conn read", slog.Any("err", err), slog.String("id", connid))
				return
			}
			logf("wswrite", connid, msg)

			if err = ws.WriteMessage(mt, msg); err != nil {
				slog.Error("ws write", slog.Any("err", err), slog.String("id", connid))
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
				slog.Error("ws read", slog.Any("err", err), slog.String("id", connid))
				return
			}
			logf("wsread", connid, msg)

			if err = conn.WriteMessage(mt, msg); err != nil {
				slog.Error("conn write", slog.Any("err", err), slog.String("id", connid))
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
