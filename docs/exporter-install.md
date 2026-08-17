# Installing btp-exporter

## Prerequisites

- [Go](https://go.dev/dl/) 1.21+
- [BTP CLI](https://tools.hana.ondemand.com/#cloud) installed and on your `PATH`

## Clone and build

```bash
git clone https://github.com/sap/crossplane-provider-btp.git
cd crossplane-provider-btp
```

Build the `btp-exporter` binary:

```bash
go build -o btp-exporter ./cmd/exporter
```

Verify it works:

```bash
./btp-exporter --help
```

## (Optional) Install to PATH

```bash
mv btp-exporter /usr/local/bin/
```

Then call it from anywhere:

```bash
btp-exporter --help
```
