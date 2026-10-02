package main

import (
	"fmt"
	"os"
	"time"
)

func main() {
	hostname, err := os.Hostname()
	if err != nil {
		hostname = "Ghost-Unknown"
	}

	if len(os.Args) > 1 {
		hostname = os.Args[1]
	} else { //add change hostname option to avoid self echo check (for testing)
		var choice string
		fmt.Printf("Do you want to change your host name? You are '%s' [y/n]: ", hostname)
		fmt.Scanln(&choice)

		if choice == "y" || choice == "Y" {
			fmt.Print("Enter new host name: ")
			fmt.Scanln(&hostname)
		}
	}
	const discoveryPort = 8888

	fmt.Printf("=== GHOST-2-GHOST CHAT ===\n")
	fmt.Printf("My Hostname: %s\n\n", hostname)

	//run listener in the background
	go ListenBroadcast(discoveryPort, hostname)
	//send discovery broadcasts every 5 seconds
	ticker := time.NewTicker(5 * time.Second)
	defer ticker.Stop()

	Broadcast(discoveryPort, hostname)

	for range ticker.C {
		Broadcast(discoveryPort, hostname)
	}
}
