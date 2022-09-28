package main

import (
	"net"
	"net/http"
	"sync"
	"time"
)

var c *http.Client
var once sync.Once

func getHTTPClient() *http.Client {
	if c == nil {
		once.Do(func() {
			netTransport := &http.Transport{
				Dial: (&net.Dialer{
					Timeout:   time.Second,
					KeepAlive: 0,
				}).Dial,
				TLSHandshakeTimeout: 5 * time.Second,
				IdleConnTimeout:     0,
				MaxIdleConnsPerHost: 50000,
				MaxIdleConns:        50000,
			}

			c = &http.Client{
				Timeout:   5 * time.Second,
				Transport: netTransport,
			}
		})
	}

	return c

}
