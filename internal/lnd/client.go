package lnd

import (
	"context"
	"crypto/x509"
	"encoding/hex"
	"fmt"
	"os"

	"github.com/lightningnetwork/lnd/lnrpc"
	"github.com/lightningnetwork/lnd/lnrpc/routerrpc"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials"
	"gopkg.in/macaroon.v2"
)

const (
	tlsCertPath  = "/certs/tls.cert"
	macaroonPath = "/macaroons/admin.macaroon"
)

// Client wraps the LND gRPC connection and exposes the methods we need.
type Client struct {
	conn      *grpc.ClientConn
	lightning lnrpc.LightningClient
	router    routerrpc.RouterClient
}

// Connect establishes a gRPC connection to LND using TLS and macaroon auth.
func Connect(host string) (*Client, error) {
	// Load TLS cert
	tlsCert, err := os.ReadFile(tlsCertPath)
	if err != nil {
		return nil, fmt.Errorf("read tls cert: %w", err)
	}
	certPool := x509.NewCertPool()
	if !certPool.AppendCertsFromPEM(tlsCert) {
		return nil, fmt.Errorf("failed to append tls cert")
	}
	tlsCreds := credentials.NewClientTLSFromCert(certPool, "")

	// Load macaroon
	macBytes, err := os.ReadFile(macaroonPath)
	if err != nil {
		return nil, fmt.Errorf("read macaroon: %w", err)
	}
	mac := &macaroon.Macaroon{}
	if err := mac.UnmarshalBinary(macBytes); err != nil {
		return nil, fmt.Errorf("unmarshal macaroon: %w", err)
	}

	// Connect
	opts := []grpc.DialOption{
		grpc.WithTransportCredentials(tlsCreds),
		grpc.WithPerRPCCredentials(&macaroonCredential{mac: mac}),
	}
	conn, err := grpc.NewClient("passthrough:///"+host, opts...)
	if err != nil {
		return nil, fmt.Errorf("connect to lnd: %w", err)
	}

	return &Client{
		conn:      conn,
		lightning: lnrpc.NewLightningClient(conn),
		router:    routerrpc.NewRouterClient(conn),
	}, nil
}

// Close closes the gRPC connection.
func (c *Client) Close() error {
	return c.conn.Close()
}

// AddInvoice creates a new invoice on LND.
// amountMsat is the invoice amount in millisatoshis.
// descriptionHash is the SHA256 hash of the LNURL metadata string (32 bytes).
// memo is an optional human-readable description stored on the invoice.
func (c *Client) AddInvoice(ctx context.Context, amountMsat int64, descriptionHash []byte, memo string) (string, error) {
	resp, err := c.lightning.AddInvoice(ctx, &lnrpc.Invoice{
		ValueMsat:       amountMsat,
		DescriptionHash: descriptionHash,
		Memo:            memo,
	})
	if err != nil {
		return "", fmt.Errorf("add invoice: %w", err)
	}
	return resp.PaymentRequest, nil
}

// SendPayment pays a BOLT-11 invoice. Returns the payment preimage on success.
func (c *Client) SendPayment(ctx context.Context, bolt11 string, maxFeeSat int64) (string, error) {
	stream, err := c.router.SendPaymentV2(ctx, &routerrpc.SendPaymentRequest{
		PaymentRequest: bolt11,
		TimeoutSeconds: 60,
		FeeLimitSat:    maxFeeSat,
		MaxParts:       16,
	})
	if err != nil {
		return "", fmt.Errorf("send payment: %w", err)
	}

	for {
		payment, err := stream.Recv()
		if err != nil {
			return "", fmt.Errorf("recv payment update: %w", err)
		}
		switch payment.Status {
		case lnrpc.Payment_SUCCEEDED:
			return payment.PaymentPreimage, nil
		case lnrpc.Payment_FAILED:
			return "", fmt.Errorf("payment failed: %s", payment.FailureReason)
			// IN_FLIGHT: continue waiting
		}
	}
}

// SubscribeInvoices opens a stream of invoice updates from LND.
// The callback is called for each settled invoice with the amount in msats
// and the description hash.
func (c *Client) SubscribeInvoices(ctx context.Context, onSettled func(amountMsat int64, descriptionHash []byte, memo string)) error {
	stream, err := c.lightning.SubscribeInvoices(ctx, &lnrpc.InvoiceSubscription{
		AddIndex:    0,
		SettleIndex: 0,
	})
	if err != nil {
		return fmt.Errorf("subscribe invoices: %w", err)
	}

	for {
		invoice, err := stream.Recv()
		if err != nil {
			return fmt.Errorf("recv invoice: %w", err)
		}
		if invoice.State == lnrpc.Invoice_SETTLED {
			onSettled(invoice.AmtPaidMsat, invoice.DescriptionHash, invoice.Memo)
		}
	}
}

// macaroonCredential implements credentials.PerRPCCredentials.
type macaroonCredential struct {
	mac *macaroon.Macaroon
}

func (m *macaroonCredential) GetRequestMetadata(ctx context.Context, uri ...string) (map[string]string, error) {
	macBytes, err := m.mac.MarshalBinary()
	if err != nil {
		return nil, err
	}
	return map[string]string{
		"macaroon": hex.EncodeToString(macBytes),
	}, nil
}

func (m *macaroonCredential) RequireTransportSecurity() bool {
	return true
}
