// Command fakeengine imitates the laya.cpp HTTP server for sidecar tests.
package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"
)

func main() {
	port := flag.Int("port", 8080, "")
	host := flag.String("host", "127.0.0.1", "")
	model := flag.String("model", "", "")
	variant := flag.String("variant", "english", "")
	delay := flag.Duration("ready-delay", 0, "")
	crash := flag.Bool("crash", false, "exit immediately")
	flag.Bool("server", false, "")
	flag.Bool("vulkan", false, "")
	flag.Bool("cuda", false, "")
	flag.Bool("cpu", false, "")
	flag.Bool("tensor-core-fp32", false, "")
	flag.Bool("flash-fp32", false, "")
	flag.Int("max-questions", 8, "")
	flag.Int("max-pending-requests", 32, "")
	flag.Int("batch-wait-ms", 2, "")
	flag.Parse()

	if *crash || os.Getenv("FAKEENGINE_CRASH") == "1" {
		fmt.Fprintln(os.Stderr, "fakeengine: simulated crash")
		os.Exit(3)
	}
	time.Sleep(*delay)
	backend := "CPU"
	for _, a := range os.Args {
		if a == "--vulkan" {
			backend = "Vulkan0"
		}
		if a == "--cuda" {
			backend = "CUDA0"
		}
	}
	mux := http.NewServeMux()
	mux.HandleFunc("/health", func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode(map[string]any{
			"status": "ok", "model": "laya-" + *variant, "variant": *variant, "backend": backend,
			"max_questions": 8, "pending_requests": 0, "queued_requests": 0, "batching": true,
			"max_batch_questions": 8, "max_pending_requests": 32, "batch_wait_ms": 2,
		})
	})
	mux.HandleFunc("/predict", func(w http.ResponseWriter, r *http.Request) {
		var reqs []map[string]any
		json.NewDecoder(r.Body).Decode(&reqs)
		results := make([]map[string]any, 0, len(reqs))
		for range reqs {
			results = append(results, map[string]any{"model": "laya-rl-agent", "answers": map[string]any{}, "usage": map[string]int{"input_tokens": 1}})
		}
		json.NewEncoder(w).Encode(map[string]any{"results": results, "elapsed_ms": 1, "backend": backend})
	})
	srv := &http.Server{Addr: fmt.Sprintf("%s:%d", *host, *port), Handler: mux}
	go func() {
		sig := make(chan os.Signal, 1)
		signal.Notify(sig, os.Interrupt, syscall.SIGTERM)
		<-sig
		srv.Close()
	}()
	fmt.Printf("Ready: %s (fake) model=%s\n", backend, strings.TrimSpace(*model))
	if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
