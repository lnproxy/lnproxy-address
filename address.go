package address

import (
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"net/url"
	"strconv"

	"github.com/lnproxy/lnproxy-client"
)

type InvoiceMaker interface {
	MakeInvoice(amount_msat uint64, description_hash []byte) (string, error)
}

type LNURLP struct {
	Domain     url.URL
	UserName   string
	MaxAmtMsat uint64
	MinAmtMsat uint64
}

func (lnurl *LNURLP) Metadata() string {
	return fmt.Sprintf(
		`[["text/plain","pay %s@%s"],["text/identifier","%s@%s"]]`,
		lnurl.UserName, lnurl.Domain.Host, lnurl.UserName, lnurl.Domain.Host,
	)
}

func (lnurl *LNURLP) JSONResponse() []byte {
	r, _ := json.Marshal(struct {
		Callback   string `json:"callback"`
		MaxAmtMsat uint64 `json:"maxSendable"`
		MinAmtMsat uint64 `json:"minSendable"`
		Metadata   string `json:"metadata"`
		Tag        string `json:"tag"`
	}{
		Callback:   lnurl.Domain.JoinPath(".well-known", "lnurlp", lnurl.UserName).String(),
		MaxAmtMsat: lnurl.MaxAmtMsat,
		MinAmtMsat: lnurl.MinAmtMsat,
		Metadata:   lnurl.Metadata(),
		Tag:        "payRequest",
	})
	return r
}

func MakeLUD6Handler(lnurl *LNURLP, invoiceMaker InvoiceMaker, lnproxy *client.LNProxy) func(w http.ResponseWriter, r *http.Request) {
	return func(w http.ResponseWriter, r *http.Request) {
		amount_string := r.URL.Query().Get("amount")
		if amount_string == "" {
			w.Write(lnurl.JSONResponse())
			return
		}
		amount_msat, err := strconv.ParseUint(amount_string, 10, 64)
		if err != nil || amount_msat < lnurl.MinAmtMsat || amount_msat > lnurl.MaxAmtMsat {
			fmt.Fprintf(w, `{"status": "ERROR", "reason": "invalid amount"}`)
			return
		}
		h := sha256.New()
		h.Write([]byte(lnurl.Metadata()))
		var routing_msat uint64
		if lnproxy != nil {
			routing_msat = lnproxy.BaseMsat + (lnproxy.Ppm*amount_msat)/1_000_000
		}
		inv, err := invoiceMaker.MakeInvoice(amount_msat-routing_msat, h.Sum(nil))
		if err != nil {
			log.Println("error requesting invoice:", err)
			fmt.Fprintf(w, `{"status": "ERROR", "reason": "error requesting invoice"}`)
			return
		}
		if lnproxy != nil {
			inv, err = lnproxy.RequestProxy(inv, routing_msat)
			if err != nil {
				log.Println("error wrapping invoice:", err)
				fmt.Fprintf(w, `{"status": "ERROR", "reason": "error wrapping invoice"}`)
				return
			}
		}
		fmt.Fprintf(w, `{"pr": "%s", "routes": []}`, inv)
	}
}
