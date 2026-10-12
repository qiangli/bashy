"""Mock OTLP/HTTP collector for the no-egress gate.

Listens ONLY on 127.0.0.1:31415 (bashy's local collector endpoint) inside the
sandbox net namespace and logs every request line to stdout. The no-egress
test asserts the gateway talks to nothing except localhost; this mock proves
the local-collector path works while the strace analysis proves nothing else
was dialled.
"""

import sys
from http.server import BaseHTTPRequestHandler, HTTPServer


class Handler(BaseHTTPRequestHandler):
    def _handle(self):
        length = int(self.headers.get("Content-Length", 0))
        if length:
            self.rfile.read(length)
        print(f"OTLP-MOCK {self.command} {self.path}", flush=True)
        body = b"{}"
        self.send_response(200)
        self.send_header("Content-Type", "application/json")
        self.send_header("Content-Length", str(len(body)))
        self.end_headers()
        self.wfile.write(body)

    do_POST = _handle
    do_GET = _handle

    def log_message(self, *args):
        pass


if __name__ == "__main__":
    port = int(sys.argv[1]) if len(sys.argv) > 1 else 31415
    server = HTTPServer(("127.0.0.1", port), Handler)
    print(f"OTLP-MOCK listening on 127.0.0.1:{port}", flush=True)
    server.serve_forever()
