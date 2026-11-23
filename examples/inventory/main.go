package main

import (
	"context"
	"fmt"
	"log"
	"os"
	"time"

	"github.com/rednote-infra/redfish-sdk-go/client"
	"github.com/rednote-infra/redfish-sdk-go/pointer"
)

func main() {
	sdk, err := client.NewClient(os.Getenv("BMC_IP"), os.Getenv("BMC_USER"), os.Getenv("BMC_PASSWORD"))
	if err != nil {
		log.Fatal(err)
	}
	defer sdk.Close()
	ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
	defer cancel()
	systems, err := sdk.GetSystems(ctx)
	if err != nil {
		log.Fatal(err)
	}
	for _, system := range systems {
		fmt.Printf("%s: %s %s (%s)\n", pointer.Value(system.Name), pointer.Value(system.Manufacturer), pointer.Value(system.Model), pointer.Value(system.PowerState))
	}
}
