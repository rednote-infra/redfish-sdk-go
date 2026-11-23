package main

import (
	"context"
	"fmt"
	"log"
	"os"
	"time"

	redfish "github.com/rednote-infra/redfish-sdk-go"
)

func main() {
	client, err := redfish.NewClient(os.Getenv("BMC_IP"), os.Getenv("BMC_USER"), os.Getenv("BMC_PASSWORD"))
	if err != nil {
		log.Fatal(err)
	}
	defer client.Close()
	ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
	defer cancel()
	systems, err := client.GetSystems(ctx)
	if err != nil {
		log.Fatal(err)
	}
	for _, system := range systems {
		fmt.Printf("%s: %s %s (%s)\n", redfish.Value(system.Name), redfish.Value(system.Manufacturer), redfish.Value(system.Model), redfish.Value(system.PowerState))
	}
}
