package sampler

import (
	"crypto/tls"
	"crypto/x509"
	"fmt"
	"os"

	"go.temporal.io/sdk/client"
)

// DialFromEnv connects to a cluster using the env vars documented in
// TRD_Replay_Gate.md §10: TEMPORAL_ADDRESS, TEMPORAL_NAMESPACE, and either
// TEMPORAL_API_KEY or a TEMPORAL_TLS_CERT/TEMPORAL_TLS_KEY pair. All are
// optional except TEMPORAL_ADDRESS; namespace defaults to "default".
func DialFromEnv() (client.Client, string, error) {
	address := os.Getenv("TEMPORAL_ADDRESS")
	if address == "" {
		return nil, "", fmt.Errorf("TEMPORAL_ADDRESS is required (see TRD_Replay_Gate.md §10)")
	}
	namespace := os.Getenv("TEMPORAL_NAMESPACE")
	if namespace == "" {
		namespace = "default"
	}

	opts := client.Options{HostPort: address, Namespace: namespace}

	switch {
	case os.Getenv("TEMPORAL_API_KEY") != "":
		opts.Credentials = client.NewAPIKeyStaticCredentials(os.Getenv("TEMPORAL_API_KEY"))
	case os.Getenv("TEMPORAL_TLS_CERT") != "" || os.Getenv("TEMPORAL_TLS_KEY") != "":
		cert, err := tls.LoadX509KeyPair(os.Getenv("TEMPORAL_TLS_CERT"), os.Getenv("TEMPORAL_TLS_KEY"))
		if err != nil {
			return nil, "", fmt.Errorf("loading TLS cert/key: %w", err)
		}
		tlsConfig := &tls.Config{Certificates: []tls.Certificate{cert}}
		if caFile := os.Getenv("TEMPORAL_TLS_CA"); caFile != "" {
			ca, err := os.ReadFile(caFile)
			if err != nil {
				return nil, "", fmt.Errorf("reading TEMPORAL_TLS_CA: %w", err)
			}
			pool := x509.NewCertPool()
			if !pool.AppendCertsFromPEM(ca) {
				return nil, "", fmt.Errorf("no certificates found in TEMPORAL_TLS_CA")
			}
			tlsConfig.RootCAs = pool
		}
		opts.ConnectionOptions.TLS = tlsConfig
	}

	c, err := client.Dial(opts)
	if err != nil {
		return nil, "", fmt.Errorf("dialing %s: %w", address, err)
	}
	return c, namespace, nil
}
