#!/bin/bash
# Generate CA
openssl genrsa -out ca.key 4096
openssl req -new -x509 -days 3650 -key ca.key -out ca.crt \
  -subj "/CN=Aether CA/O=Development/C=US"

# Generate server cert
openssl genrsa -out server.key 4096
openssl req -new -key server.key -out server.csr \
  -subj "/CN=localhost/O=Aether/C=US"
openssl x509 -req -days 365 -in server.csr \
  -CA ca.crt -CAkey ca.key -CAcreateserial -out server.crt
rm server.csr

echo "Generated: ca.crt, ca.key, server.crt, server.key"
