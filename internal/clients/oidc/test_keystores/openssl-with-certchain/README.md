# OpenSSL Test Keystore with Certificate Chain

This directory contains test certificates for validating certificate chain parsing functionality. It is based on the `../openssl` sample and reuse its key.pem and subject and ketstore.pass.txt. It is used to check the functionality of parseCert(containsCertificateChain=True) with the parsing of the full certificate chain.

## Files

- **`key.pem`** — Private key (reused from `../openssl/key.pem`) used for ALL certificate authorities.
- **`root.pem`** — Self-signed root CA certificate
- **`int.pem`** — Intermediate CA certificate (signed by root)
- **`leaf.pem`** — End-entity certificate (signed by intermediate)
- **`cert.pem`** — Full certificate chain (leaf + intermediate + root concatenated) 
- **`keystore.p12`** — PKCS#12 bundle containing the private key and full certificate chain
- **`keystore.pass.txt`** — Password for the PKCS#12 file

## Certificate Chain Structure

```
Root CA (self-signed)
  └─> Intermediate CA
       └─> Leaf Certificate
```

## Key Characteristics

- **Same private key** used for all certificates (root CA, intermediate CA, and leaf) — acceptable for unit tests only
- Root CA: `basicConstraints=CA:TRUE`, `keyUsage=keyCertSign,cRLSign` `Subject: C=AU, ST=test, L=test, O=Test Root, CN=Test Root` `Issuer: C=AU, ST=test, L=test, O=Test Root, CN=Test Root`
- Intermediate CA: `basicConstraints=CA:TRUE`, `keyUsage=keyCertSign,cRLSign` `Subject: C=AU, ST=test, L=test, O=Test Int, CN=Test Intermediate` `Issuer: C=AU, ST=test, L=test, O=Test Root, CN=Test Root`
- Leaf cert: `basicConstraints=CA:FALSE`, `keyUsage=digitalSignature,keyEncipherment` `Subject: C=AU, ST=test, L=test, O=test, CN=test` `Issuer: C=AU, ST=test, L=test, O=Test Int, CN=Test Intermediate`

## Reading Certificates

```bash
# View individual certificate details
openssl x509 -in root.pem -noout -text
openssl x509 -in int.pem -noout -text
openssl x509 -in leaf.pem -noout -text

# View PKCS#12 contents (password in keystore.pass.txt)
openssl pkcs12 -info -in keystore.p12

# Verify chain
openssl verify -CAfile root.pem -untrusted int.pem leaf.pem
```

## How It Was Created

```bash

# Create self-signed root CA (using same private key)
openssl req -x509 -new -key key.pem -days 1 -out root.pem \
  -subj "/C=AU/ST=test/L=test/O=Test Root/CN=Test Root" \
  -addext "basicConstraints = CA:TRUE" \
  -addext "keyUsage = keyCertSign, cRLSign"

# Create intermediate CA CSR and sign with root
openssl req -new -key key.pem -out int.csr \
  -subj "/C=AU/ST=test/L=test/O=Test Int/CN=Test Intermediate"
openssl x509 -req -in int.csr -CA root.pem -CAkey key.pem -CAcreateserial \
  -out int.pem -days 1 -sha256 \
  -extfile <(printf "basicConstraints=CA:TRUE\nkeyUsage=keyCertSign,cRLSign")

# Create leaf certificate CSR and sign with intermediate
openssl req -new -key key.pem -out leaf.csr \
  -subj "/C=AU/ST=test/L=test/O=test/CN=test"
openssl x509 -req -in leaf.csr -CA int.pem -CAkey key.pem -CAcreateserial \
  -out leaf.pem -days 1 -sha256 \
  -extfile <(printf "basicConstraints=CA:FALSE\nkeyUsage=digitalSignature,keyEncipherment\nsubjectAltName=DNS:test")

# Create full chain (leaf + intermediate + root)
cat leaf.pem int.pem root.pem > cert.pem

# Create PKCS#12 with private key and full chain
cat int.pem root.pem > chain.pem
openssl pkcs12 -export -in leaf.pem -inkey key.pem \
  -certfile chain.pem -out keystore.p12
```

## Purpose

Used by `certlogin_test.go` to test certificate parsing with intermediate CA chains.
