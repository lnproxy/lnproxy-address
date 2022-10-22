package main

import (
	"bytes"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"regexp"
	"strconv"
	"strings"

	"github.com/fiatjaf/makeinvoice"
)

var (
	httpPort        = flag.String("http-port", "4760", "http port over which to expose address server")
	domain          = flag.String("domain", "example.com", "domain of LN SERVICE")
	lnproxyURL      = flag.String("lnproxy-url", "https://lnproxy.org/api/", "REST host for lnproxy")
	lnproxyBaseMsat = flag.Uint64("lnproxy-routing-base", 1000, "base routing budget for lnproxy")
	lnproxyPpmMsat  = flag.Uint64("lnproxy-routing-ppm", 6000, "ppm routing budget for lnproxy")
	lnproxyClient   *http.Client
)

type UserParams struct {
	UserName   string
	MaxAmtMsat uint64
	MinAmtMsat uint64
	NodeType   string
}

func (u UserParams) metadata() string {
	return fmt.Sprintf(
		"[[\"text/plain\",\"pay %s@%s\"],[\"text/identifier\",\"%s@%s\"]]",
		u.UserName, *domain, u.UserName, *domain)
}

func (u UserParams) toResponse() ([]byte, error) {
	return json.Marshal(struct {
		Callback   string `json:"callback"`
		MaxAmtMsat uint64 `json:"maxSendable"`
		MinAmtMsat uint64 `json:"minSendable"`
		Metadata   string `json:"metadata"`
		Tag        string `json:"tag"`
	}{
		Callback:   fmt.Sprintf("https://%s/.well-known/lnurlp/%s", *domain, u.UserName),
		MaxAmtMsat: u.MaxAmtMsat,
		MinAmtMsat: u.MinAmtMsat,
		Metadata:   u.metadata(),
		Tag:        "payRequest",
	})
}

func wrap(bolt11 string, routing_msat uint64) (string, error) {
	req, err := http.NewRequest("GET", fmt.Sprintf("%s/%s?routing_msat=%d", *lnproxyURL, bolt11, routing_msat), nil)
	if err != nil {
		return "", err
	}
	resp, err := lnproxyClient.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	buf := new(strings.Builder)
	_, err = io.Copy(buf, resp.Body)
	if err != nil {
		return "", err
	}
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("lnproxy error: %s", buf.String())
	}
	wbolt11 := strings.TrimSpace(buf.String())
	a, h, err := extractInvoiceDetails([]byte(bolt11))
	if err != nil {
		return "", err
	}
	wa, wh, err := extractInvoiceDetails([]byte(wbolt11))
	if err != nil {
		return "", err
	}
	if bytes.Compare(h, wh) != 0 {
		return "", fmt.Errorf("Wrapped payment hash does not match!")
	}
	if (a + routing_msat) != wa {
		return "", fmt.Errorf("Wrapped routing budget too high!")
	}
	return wbolt11, nil
}

var CharSet = []byte("qpzry9x8gf2tvdw0s3jn54khce6mua7l")

var validInvoice = regexp.MustCompile("^lnbc(?:[0-9]+[pnum])?1[qpzry9x8gf2tvdw0s3jn54khce6mua7l]+$")

func extractInvoiceDetails(invoice []byte) (uint64, []byte, error) {
	invoice = bytes.ToLower(invoice)
	pos := bytes.LastIndexByte(invoice, byte('1'))
	if pos == -1 || !validInvoice.Match(invoice) {
		return 0, nil, fmt.Errorf("Invalid invoice")
	}

	var msat uint64
	var err error
	if pos > 4 {
		msat, err = strconv.ParseUint(string(invoice[4:pos-1]), 10, 64)
		if err != nil {
			return 0, nil, err
		}
		switch invoice[pos-1] {
		case byte('p'):
			msat = msat / 10
		case byte('n'):
			msat = msat * 100
		case byte('u'):
			msat = msat * 100_000
		case byte('m'):
			msat = msat * 100_000_000
		}
	}
	for i := pos + 8; i < len(invoice); {
		if bytes.Compare(invoice[i:i+3], []byte("pp5")) == 0 {
			return msat, invoice[i+1+2 : i+1+2+52], nil
		}
		i += 3 + bytes.Index(CharSet, invoice[i+1:i+2])*32 + bytes.Index(CharSet, invoice[i+2:i+3])
	}
	return 0, nil, fmt.Errorf("No 'p' tag")
}

var validPath = regexp.MustCompile("^/.well-known/lnurlp/([a-z0-9-_.]+)$")

func lud6Handler(w http.ResponseWriter, r *http.Request) {
	m := validPath.FindStringSubmatch(r.URL.Path)
	if m == nil {
		log.Println(r.URL.Path)
		fmt.Fprintf(w, `{"status": "ERROR", "reason": "invalid username"}`)
		return
	}
	userParamsBytes, err := os.ReadFile("user/" + m[1])
	if err != nil {
		log.Println(err)
		fmt.Fprintf(w, `{"status": "ERROR", "reason": "username is not registered"}`)
		return
	}
	userParams := UserParams{}
	err = json.Unmarshal(userParamsBytes, &userParams)
	if err != nil {
		log.Println(err)
		fmt.Fprintf(w, `{"status": "ERROR", "reason": "user details corrupted"}`)
		return
	}
	amt_string := r.URL.Query().Get("amount")
	if amt_string == "" {
		resp, err := userParams.toResponse()
		if err != nil {
			log.Println(err)
			fmt.Fprintf(w, `{"status": "ERROR", "reason": "user details corrupted"}`)
			return
		}
		w.Write(resp)
		return
	}
	amt_msat, err := strconv.ParseUint(amt_string, 10, 64)
	if err != nil || amt_msat < userParams.MinAmtMsat || amt_msat > userParams.MaxAmtMsat {
		log.Println(err)
		fmt.Fprintf(w, `{"status": "ERROR", "reason": "invalid amount"}`)
		return
	}
	userBytes, err := os.ReadFile("node/" + m[1])
	if err != nil {
		log.Println(err)
		fmt.Fprintf(w, `{"status": "ERROR", "reason": "user details corrupted"}`)
		return
	}
	var inv string
	routing_msat := *lnproxyBaseMsat + (*lnproxyPpmMsat*amt_msat)/1_000_000
	switch userParams.NodeType {
	case "Commando":
		userNode := makeinvoice.CommandoParams{}
		err = json.Unmarshal(userBytes, &userNode)
		if err != nil {
			log.Println(err)
			fmt.Fprintf(w, `{"status": "ERROR", "reason": "user details corrupted"}`)
			return
		}
		inv, err = makeinvoice.MakeInvoice(makeinvoice.Params{
			Backend:     userNode,
			Msatoshi:    int64(amt_msat - routing_msat),
			Description: userParams.metadata(),
			UseDescriptionHash : true,
		})
	case "Sparko":
		userNode := makeinvoice.SparkoParams{}
		err = json.Unmarshal(userBytes, &userNode)
		if err != nil {
			log.Println(err)
			fmt.Fprintf(w, `{"status": "ERROR", "reason": "user details corrupted"}`)
			return
		}
		inv, err = makeinvoice.MakeInvoice(makeinvoice.Params{
			Backend:     userNode,
			Msatoshi:    int64(amt_msat - routing_msat),
			Description: userParams.metadata(),
			UseDescriptionHash : true,
		})
	case "LND":
		userNode := makeinvoice.LNDParams{}
		err = json.Unmarshal(userBytes, &userNode)
		if err != nil {
			log.Println(err)
			fmt.Fprintf(w, `{"status": "ERROR", "reason": "user details corrupted"}`)
			return
		}
		inv, err = makeinvoice.MakeInvoice(makeinvoice.Params{
			Backend:     userNode,
			Msatoshi:    int64(amt_msat - routing_msat),
			Description: userParams.metadata(),
			UseDescriptionHash : true,
		})
	case "LNBits":
		userNode := makeinvoice.LNBitsParams{}
		err = json.Unmarshal(userBytes, &userNode)
		if err != nil {
			log.Println(err)
			fmt.Fprintf(w, `{"status": "ERROR", "reason": "user details corrupted"}`)
			return
		}
		inv, err = makeinvoice.MakeInvoice(makeinvoice.Params{
			Backend:     userNode,
			Msatoshi:    int64(amt_msat - routing_msat),
			Description: userParams.metadata(),
			UseDescriptionHash : true,
		})
	case "Eclair":
		userNode := makeinvoice.EclairParams{}
		err = json.Unmarshal(userBytes, &userNode)
		if err != nil {
			log.Println(err)
			fmt.Fprintf(w, `{"status": "ERROR", "reason": "user details corrupted"}`)
			return
		}
		inv, err = makeinvoice.MakeInvoice(makeinvoice.Params{
			Backend:     userNode,
			Msatoshi:    int64(amt_msat - routing_msat),
			Description: userParams.metadata(),
			UseDescriptionHash : true,
		})
	default:
		fmt.Fprintf(w, `{"status": "ERROR", "reason": "user details corrupted"}`)
		return
	}
	if err != nil {
		log.Println(err)
		fmt.Fprintf(w, `{"status": "ERROR", "reason": "error while requesting invoice"}`)
		return
	}
	winv, err := wrap(inv, routing_msat)
	if err != nil {
		log.Println(err)
		fmt.Fprintf(w, `{"status": "ERROR", "reason": "error while wrapping invoice"}`)
		return
	}
	fmt.Fprintf(w, `{"pr": "%s", "routes": []}`, winv)
}

func main() {
	flag.Parse()
	makeinvoice.Client = &http.Client{Timeout: 25 * time.Second}
	lnproxyClient = &http.Client{Timeout: 15 * time.Second}
	http.HandleFunc("/", lud6Handler)
	log.Fatal(http.ListenAndServe("localhost:"+*httpPort, nil))
}
