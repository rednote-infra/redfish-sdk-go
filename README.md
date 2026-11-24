# redfish-sdk-go

A native Go SDK for managing server BMCs through the [DMTF Redfish](https://www.dmtf.org/standards/redfish) protocol. Query hardware inventory, control power and boot settings, update firmware, and collect diagnostic logs across multiple vendors.

## Features

- **Multi-vendor hardware inventory** — inspect CPU, memory, GPU, storage, networking, power, and cooling through one client.
- **Remote power and boot control** — power servers on or off, restart them, and select PXE, disk, or BIOS setup boot.
- **BIOS configuration** — validate attribute values, update pending settings, and schedule when changes take effect.
- **Firmware updates** — use standard Redfish update actions, multipart uploads, and vendor-specific workflows.
- **Find resources quickly** — discover standard and OEM resources by type and properties without hardcoding collection paths.
- **One-call out-of-band log download** — collect diagnostic logs, wait for completion, and download the bundle to a file or memory.
- **Concurrent automation** — use goroutines and context cancellation to coordinate operations across BMCs.

The SDK uses only the Go standard library. Vendor strategies cover AMI MegaRAC, ZTE, Inspur/IEIT, H3C, xFusion, Lenovo, Nettrix, Enginetech, and smoothcompute, with support varying by operation and BMC capabilities.

## Installation

```bash
go get github.com/rednote-infra/redfish-sdk-go
```

## Quick Start

```go
package main

import (
	"context"
	"fmt"
	"log"
	"os"
	"time"

	"github.com/rednote-infra/redfish-sdk-go/client"
	"github.com/rednote-infra/redfish-sdk-go/models"
)

func main() {
	if err := run(); err != nil {
		log.Fatal(err)
	}
}

func run() error {
	sdk, err := client.NewClient(
		os.Getenv("BMC_IP"),
		os.Getenv("BMC_USER"),
		os.Getenv("BMC_PASSWORD"),
	)
	if err != nil {
		return err
	}
	defer sdk.Close()

	ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
	defer cancel()

	system, err := sdk.GetSystem(ctx)
	if err != nil {
		return err
	}
	fmt.Printf("Server: %s %s\n", models.Value(system.Manufacturer), models.Value(system.Model))
	fmt.Printf("SN: %s\nPower: %s\n", models.Value(system.SerialNumber), models.Value(system.PowerState))

	processors, err := sdk.GetProcessors(ctx)
	if err != nil {
		return err
	}
	for _, cpu := range processors {
		fmt.Printf("CPU: %s, %d cores / %d threads\n",
			models.Value(cpu.Model), models.Value(cpu.TotalCores), models.Value(cpu.TotalThreads))
	}

	memory, err := sdk.GetMemory(ctx)
	if err != nil {
		return err
	}
	for _, dimm := range memory {
		capacity := "unknown"
		if dimm.CapacityMib != nil {
			capacity = fmt.Sprintf("%.1f GiB", float64(*dimm.CapacityMib)/1024)
		}
		fmt.Printf("DIMM: %s %s\n", models.Value(dimm.Manufacturer), capacity)
	}
	return nil
}
```

See [examples/inventory](examples/inventory/main.go) for another inventory example.

## Power Control

Using `sdk` and `ctx` from the quick start:

```go
if _, err := sdk.Reset(ctx, client.ResetOn, client.ResetOptions{}); err != nil {
	return err
}
```

Other reset types include `client.ResetGracefulShutdown`, `client.ResetGracefulRestart`, and `client.ResetForceRestart`. The SDK checks the action and values advertised by the BMC.

## Diagnostic Downloads

Collect, wait, and save the diagnostic bundle:

```go
logCtx, cancelLogs := context.WithTimeout(context.Background(), 10*time.Minute)
defer cancelLogs()

path, err := sdk.CollectAndDownloadDiagnosticData(
	logCtx,
	"diagnostic.tar.gz",
	client.CollectDownloadOptions{},
)
if err != nil {
	return err
}
fmt.Println("Saved:", path)
```

To return the bundle in memory:

```go
bundle, err := sdk.CollectAndDownloadDiagnosticDataBytes(
	logCtx,
	client.CollectDownloadOptions{},
)
if err != nil {
	return err
}
fmt.Printf("Downloaded %d bytes\n", len(bundle))
```

## Contributing

Issues and pull requests are welcome.

## License

[BSD 3-Clause License](LICENSE).
