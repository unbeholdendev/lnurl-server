package main

import (
	"context"
	"flag"
	"log"
	"os"
	"os/signal"
	"syscall"

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
	lndClient, err := lnd.Connect(cfg.LND.Host, cfg.LND.TLSCertPath, cfg.LND.MacaroonPath)
	if err != nil {
		log.Fatalf("failed to connect to lnd: %v", err)
	}
	defer lndClient.Close()
	log.Printf("connected to lnd at %s", cfg.LND.Host)

	// Create notifier
	ntfy := notifier.NewNtfy(cfg.Ntfy)

	// Create handlers
	payHandler := lnurl.NewPayHandler(cfg.Pay, lndClient)
	withdrawHandler := lnurl.NewWithdrawHandler(
		cfg.ExpandVouchers(),
		lndClient,
		ntfy,
		"data/state.json",
	)
	addressHandler := lnurl.NewAddressHandler(cfg.Addresses, lndClient)

	// Start invoice subscriber in background
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	go runInvoiceSubscriber(ctx, lndClient, ntfy, payHandler, addressHandler, cfg)

	// Start HTTP server
	srv := server.New(cfg, payHandler, withdrawHandler, addressHandler)

	// Handle shutdown
	go func() {
		sigCh := make(chan os.Signal, 1)
		signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)
		<-sigCh
		log.Println("shutting down...")
		cancel()
		os.Exit(0)
	}()

	if err := srv.ListenAndServe(); err != nil {
		log.Fatalf("server error: %v", err)
	}
}

func runInvoiceSubscriber(ctx context.Context, lndClient *lnd.Client, ntfy *notifier.Ntfy, payHandler *lnurl.PayHandler, addressHandler *lnurl.AddressHandler, cfg *config.Config) {
	for {
		log.Println("starting invoice subscriber...")
		err := lndClient.SubscribeInvoices(ctx, func(amountMsat int64, descriptionHash []byte, memo string) {
			amountSats := amountMsat / 1000
			source := identifyPaymentSource(descriptionHash, payHandler, addressHandler, cfg)
			log.Printf("invoice settled: %d sats via %s", amountSats, source)
			ntfy.NotifyPayment(amountSats, source)
		})

		if ctx.Err() != nil {
			return // context cancelled, shutting down
		}
		log.Printf("invoice subscriber disconnected: %v, reconnecting...", err)
	}
}

func identifyPaymentSource(descriptionHash []byte, payHandler *lnurl.PayHandler, addressHandler *lnurl.AddressHandler, cfg *config.Config) string {
	if len(descriptionHash) != 32 {
		return "unknown"
	}

	var hash [32]byte
	copy(hash[:], descriptionHash)

	// Check lnurl-pay
	if payHandler != nil {
		if hash == payHandler.MetadataHash() {
			return "lnurl-pay"
		}
	}

	// Check lightning addresses (for each allowed domain)
	if addressHandler != nil {
		for _, domain := range cfg.Server.AllowedDomains {
			hashMap := addressHandler.MetadataHashForHost(domain)
			if source, ok := hashMap[hash]; ok {
				return source
			}
		}
	}

	return "unknown"
}
