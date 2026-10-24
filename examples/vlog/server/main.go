// vlog-server serves a built vlog site from a directory on loopback, so a
// paired Outpost can publish it as a link. Standard library only.
//
//	vlog-server -dir public -addr 127.0.0.1:18090
package main

import (
	"flag"
	"log"
	"net/http"
	"time"
)

func main() {
	dir := flag.String("dir", "public", "site directory to serve")
	addr := flag.String("addr", "127.0.0.1:18090", "listen address (keep it loopback; Outpost publishes it)")
	flag.Parse()

	files := http.FileServer(http.Dir(*dir))
	h := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("X-Content-Type-Options", "nosniff")
		w.Header().Set("Referrer-Policy", "no-referrer")
		files.ServeHTTP(w, r)
	})
	srv := &http.Server{Addr: *addr, Handler: h, ReadHeaderTimeout: 10 * time.Second}
	log.Printf("vlog-server: serving %s on http://%s/", *dir, *addr)
	log.Fatal(srv.ListenAndServe())
}
