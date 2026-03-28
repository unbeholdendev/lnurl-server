package main

import (
	"context"
	"flag"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"lnurl-server/internal/config"
	"lnurl-server/internal/lnd"
	"lnurl-server/internal/lnurl"
	"lnurl-server/internal/notifier"
	"lnurl-server/internal/server"
)

func main() {
	configPath := flag.String("config", "config.yaml", "path to config file")
	flag.Parse()

	// Load config
	cfg, err := config.Load(*configPath)
	if err != nil {
		log.Fatalf("failed to load config: %v", err)
	}
	log.Printf("config loaded from %s", *configPath)

	// Connect to LND
	lndClient, err := lnd.Connect(cfg.LND.Host)
	if err != nil {
		log.Fatalf("failed to connect to lnd: %v", err)
	}
	defer lndClient.Close()
	log.Printf("connected to lnd at %s", cfg.LND.Host)

	// Create notifier
	ntfy := notifier.NewNtfy(cfg.Ntfy)

	// Create handlers
	withdrawHandler := lnurl.NewWithdrawHandler(
		cfg.ExpandVouchers(),
		lndClient,
		ntfy,
		"data/state.json",
	)
	addressHandler := lnurl.NewAddressHandler(cfg.Addresses, lndClient, ntfy)

	// Print unclaimed withdraw voucher URLs
	if withdrawHandler != nil {
		unclaimed := withdrawHandler.UnclaimedVouchers()
		log.Printf("withdraw vouchers: %d unclaimed", len(unclaimed))
		for _, v := range unclaimed {
			log.Printf("  %s (%d sats) %s", v.ID, v.Amount/1000, v.URL)
		}
	}

	// Start invoice subscriber in background
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	if withdrawHandler != nil {
		go withdrawHandler.StartCleanup(ctx)
	}

	go runInvoiceSubscriber(ctx, lndClient, ntfy)

	// Start HTTP server
	srv := server.New(cfg, withdrawHandler, addressHandler)

	// Handle shutdown signals
	go func() {
		sigCh := make(chan os.Signal, 1)
		signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)
		sig := <-sigCh
		log.Printf("received signal %v, shutting down...", sig)

		// Cancel background goroutines
		cancel()

		// Give in-flight requests up to 30 seconds to complete
		shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer shutdownCancel()

		if err := srv.Shutdown(shutdownCtx); err != nil {
			log.Printf("shutdown error: %v", err)
		}
	}()

	if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
		log.Fatalf("server error: %v", err)
	}

	log.Println("server stopped")
}

func runInvoiceSubscriber(ctx context.Context, lndClient *lnd.Client, ntfy *notifier.Ntfy) {
	backoff := 1 * time.Second
	const maxBackoff = 30 * time.Second

	for {
		log.Println("starting invoice subscriber...")
		start := time.Now()
		err := lndClient.SubscribeInvoices(ctx, func(amountMsat int64, descriptionHash []byte, memo string) {
			amountSats := amountMsat / 1000
			log.Printf("invoice settled: %d sats", amountSats)
			ntfy.NotifyPayment(amountSats, memo)
		})

		if ctx.Err() != nil {
			return // context cancelled, shutting down
		}

		// Reset backoff if the connection lasted more than a minute
		if time.Since(start) > time.Minute {
			backoff = 1 * time.Second
		}

		log.Printf("invoice subscriber disconnected: %v, reconnecting in %s...", err, backoff)

		select {
		case <-ctx.Done():
			return
		case <-time.After(backoff):
		}

		backoff = min(backoff*2, maxBackoff)
	}
}
