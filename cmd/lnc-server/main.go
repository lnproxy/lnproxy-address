package main

import (
	"crypto/tls"
	"crypto/x509"
	"encoding/hex"
	"flag"
	"fmt"
	"log"
	"net/http"
	"net/url"
	"os"
	"time"

	"github.com/lnproxy/lnc"
	"github.com/lnproxy/lnproxy-address"
	"github.com/lnproxy/lnproxy-client"
)

type LncInvoicer struct {
	lnc.LN
}

func (invoicer *LncInvoicer) MakeInvoice(amount_msat uint64, description_hash []byte) (string, error) {
	return invoicer.AddInvoice(lnc.InvoiceParameters{
		DescriptionHash: description_hash,
		ValueMsat:       amount_msat,
	})
}

func main() {
	domain := flag.String("domain", "example.com", "domain of LN SERVICE")
	username := flag.String("username", "_", "lud6 username")
	maxAmtMsat := flag.Uint64("max-msat", 10_000_000_000, "max msat per payment")
	minAmtMsat := flag.Uint64("min-msat", 10_000, "min msat per payment")
	httpPort := flag.String("port", "4747", "http port over which to expose api")
	lndHostString := flag.String("lnd", "https://127.0.0.1:8080", "host for lnd's REST api")
	lndCertPath := flag.String(
		"lnd-cert",
		".lnd/tls.cert",
		"lnd's self-signed cert (set to empty string for no-rest-tls=true)",
	)
	lnproxyUrlString := flag.String("lnproxy-url", "https://lnproxy.org/spec/", "REST API url for lnproxy relay, empty string for no proxying")
	lnproxyBaseMsat := flag.Uint64("lnproxy-routing-base", 2_000, "base routing budget for lnproxy relay")
	lnproxyPpm := flag.Uint64("lnproxy-routing-ppm", 10_000, "ppm routing budget for lnproxy relay")

	flag.Usage = func() {
		fmt.Fprintf(flag.CommandLine.Output(), `usage: %s [flags] address.macaroon
  address.macaroon
	Path to address macaroon. Generate it with:
		lncli bakemacaroon --save_to address.macaroon \
			uri:/invoicesrpc.Invoices/AddInvoice
`, os.Args[0])
		flag.PrintDefaults()
		os.Exit(2)
	}

	flag.Parse()
	if len(flag.Args()) != 1 {
		flag.Usage()
		os.Exit(2)
	}

	macaroonBytes, err := os.ReadFile(flag.Args()[0])
	if err != nil {
		log.Fatalln("unable to read lnproxy macaroon file:", err)
	}
	macaroon := hex.EncodeToString(macaroonBytes)

	lndHost, err := url.Parse(*lndHostString)
	if err != nil {
		log.Fatalln("unable to parse lnd host url:", err)
	}

	var lndTlsConfig *tls.Config
	if *lndCertPath == "" {
		lndTlsConfig = &tls.Config{}
	} else {
		lndCert, err := os.ReadFile(*lndCertPath)
		if err != nil {
			log.Fatalln("unable to read lnd tls certificate file:", err)
		}
		caCertPool := x509.NewCertPool()
		caCertPool.AppendCertsFromPEM(lndCert)
		lndTlsConfig = &tls.Config{RootCAs: caCertPool}
	}

	lndClient := &http.Client{
		Transport: &http.Transport{
			TLSClientConfig: lndTlsConfig,
		},
	}

	domainURL, err := url.Parse(*domain)
	if err != nil {
		log.Fatalln("unable to parse domain url:", err)
	}

	var lnproxy *client.LNProxy
	if *lnproxyUrlString != "" {
		lnproxyUrl, err := url.Parse(*lnproxyUrlString)
		if err != nil {
			log.Fatalln("unable to parse lnproxy url:", err)
		}
		lnproxy = &client.LNProxy{
			URL:      *lnproxyUrl,
			Client:   http.Client{Timeout: 15 * time.Second},
			BaseMsat: *lnproxyBaseMsat,
			Ppm:      *lnproxyPpm,
		}
	}

	http.HandleFunc("/.well-known/lnurlp/", address.MakeLUD6Handler(
		&address.LNURLP{
			Domain:     *domainURL,
			UserName:   *username,
			MaxAmtMsat: *maxAmtMsat,
			MinAmtMsat: *minAmtMsat,
		},
		&LncInvoicer{
			LN: &lnc.Lnd{
				Host:      lndHost,
				Client:    lndClient,
				TlsConfig: lndTlsConfig,
				Macaroon:  macaroon,
			},
		},
		lnproxy,
	))
	log.Fatalln(http.ListenAndServe("localhost:"+*httpPort, nil))
}
