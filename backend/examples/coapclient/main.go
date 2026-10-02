// Command coapclient is a tiny example/verification client for the CoAP
// gateway. It POSTs a property report to a device resource.
//
// Usage:
//
//	go run ./examples/coapclient \
//	  -addr 127.0.0.1:5683 -workspace factory1 \
//	  -device sensor-a -secret <secret> \
//	  -payload '{"temperature":33.3}'
package main

import (
	"bytes"
	"context"
	"flag"
	"fmt"
	"log"
	"os"
	"time"

	"github.com/plgd-dev/go-coap/v3/message"
	"github.com/plgd-dev/go-coap/v3/udp"
)

func main() {
	addr := flag.String("addr", "127.0.0.1:5683", "coap gateway address")
	product := flag.String("product", "sensor", "product key")
	device := flag.String("device", "sensor-a", "device key")
	secret := flag.String("secret", "", "device secret")
	kind := flag.String("kind", "properties", "resource kind: properties|events|peer")
	payload := flag.String("payload", `{"temperature":33.3}`, "json payload")
	flag.Parse()

	if *secret == "" {
		fmt.Fprintln(os.Stderr, "missing -secret")
		os.Exit(2)
	}

	conn, err := udp.Dial(*addr)
	if err != nil {
		log.Fatalf("dial: %v", err)
	}
	defer conn.Close()

	path := fmt.Sprintf("/%s/%s/%s", *product, *device, *kind)

	ctx, cancel := context.WithTimeout(context.Background(), 8*time.Second)
	defer cancel()

	req, err := conn.NewPostRequest(ctx, path, message.AppJSON, bytes.NewReader([]byte(*payload)))
	if err != nil {
		log.Fatalf("new request: %v", err)
	}
	// CoAP carries the secret as a Uri-Query option, not in the path.
	req.AddQuery("secret=" + *secret)

	resp, err := conn.Do(req)
	if err != nil {
		log.Fatalf("post: %v", err)
	}
	body, _ := resp.ReadBody()
	fmt.Printf("code=%s body=%s\n", resp.Code(), string(body))
}
