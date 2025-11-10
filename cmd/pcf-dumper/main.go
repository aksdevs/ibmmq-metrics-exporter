package main

import (
	"encoding/binary"
	"encoding/hex"
	"flag"
	"fmt"
	"log"

	"github.com/atulksin/ibmmq-go-stat-otel/pkg/config"
	"github.com/atulksin/ibmmq-go-stat-otel/pkg/mqclient"
	"github.com/sirupsen/logrus"
)

func main() {
	// Command line flags
	var configFile string
	flag.StringVar(&configFile, "c", "configs/default.yaml", "Configuration file path")
	flag.Parse()

	fmt.Println("=== IBM MQ PCF Data Dumper ===")
	fmt.Printf("Configuration loaded from: %s\n", configFile)

	// Load configuration
	cfg, err := config.LoadConfig(configFile)
	if err != nil {
		log.Fatalf("Failed to load configuration: %v", err)
	}

	// Validate configuration
	if err := cfg.Validate(); err != nil {
		log.Fatalf("Configuration validation failed: %v", err)
	}

	fmt.Printf("Queue Manager: %s\n", cfg.MQ.QueueManager)
	fmt.Printf("Connection: %s via %s\n", cfg.MQ.GetConnectionName(), cfg.MQ.Channel)
	fmt.Printf("Statistics Queue: %s\n", cfg.Collector.StatsQueue)
	fmt.Printf("Accounting Queue: %s\n", cfg.Collector.AccountingQueue)
	fmt.Println()

	// Create logger
	logger := logrus.New()
	logger.SetLevel(logrus.InfoLevel)

	// Create MQ client using the loaded configuration
	client := mqclient.NewMQClient(&cfg.MQ, logger)

	// Connect
	if err := client.Connect(); err != nil {
		log.Fatalf("Failed to connect: %v", err)
	}
	defer client.Disconnect()

	// Open queues using configuration
	if err := client.OpenStatsQueue(cfg.Collector.StatsQueue); err != nil {
		log.Printf("Failed to open statistics queue: %v", err)
	}
	if err := client.OpenAccountingQueue(cfg.Collector.AccountingQueue); err != nil {
		log.Printf("Failed to open accounting queue: %v", err)
	}

	// Get accounting messages
	fmt.Println("\n--- ACCOUNTING MESSAGES ---")
	acctMessages, err := client.GetAllMessages("accounting")
	if err != nil {
		log.Printf("Error getting accounting messages: %v", err)
	} else {
		fmt.Printf("Retrieved %d accounting messages\n", len(acctMessages))

		for i, msg := range acctMessages {
			msgData := msg.Data
			fmt.Printf("\n=== Accounting Message %d ===\n", i+1)
			fmt.Printf("Length: %d bytes\n", len(msgData))

			// Show hex dump of first 64 bytes
			fmt.Printf("Hex dump (first 64 bytes):\n")
			if len(msgData) > 64 {
				fmt.Printf("%s\n", hex.Dump(msgData[:64]))
			} else {
				fmt.Printf("%s\n", hex.Dump(msgData))
			}

			// Try to parse PCF header
			if len(msgData) >= 36 {
				fmt.Printf("PCF Header Analysis:\n")
				fmt.Printf("  Type (BE):           %d\n", binary.BigEndian.Uint32(msgData[0:4]))
				fmt.Printf("  Type (LE):           %d\n", binary.LittleEndian.Uint32(msgData[0:4]))
				fmt.Printf("  StrucLength (BE):    %d\n", binary.BigEndian.Uint32(msgData[4:8]))
				fmt.Printf("  StrucLength (LE):    %d\n", binary.LittleEndian.Uint32(msgData[4:8]))
				fmt.Printf("  Command (BE):        %d\n", binary.BigEndian.Uint32(msgData[12:16]))
				fmt.Printf("  Command (LE):        %d\n", binary.LittleEndian.Uint32(msgData[12:16]))
				fmt.Printf("  ParamCount (BE):     %d\n", binary.BigEndian.Uint32(msgData[32:36]))
				fmt.Printf("  ParamCount (LE):     %d\n", binary.LittleEndian.Uint32(msgData[32:36]))
			}

			if i >= 2 { // Limit to first 3 messages
				fmt.Printf("... (showing first 3 messages only)\n")
				break
			}
		}
	}

	// Get statistics messages
	fmt.Println("\n--- STATISTICS MESSAGES ---")
	statsMessages, err := client.GetAllMessages("stats")
	if err != nil {
		log.Printf("Error getting statistics messages: %v", err)
	} else {
		fmt.Printf("Retrieved %d statistics messages\n", len(statsMessages))

		for i, msg := range statsMessages {
			msgData := msg.Data
			fmt.Printf("\n=== Statistics Message %d ===\n", i+1)
			fmt.Printf("Length: %d bytes\n", len(msgData))

			// Show hex dump of first 64 bytes
			fmt.Printf("Hex dump (first 64 bytes):\n")
			if len(msgData) > 64 {
				fmt.Printf("%s\n", hex.Dump(msgData[:64]))
			} else {
				fmt.Printf("%s\n", hex.Dump(msgData))
			}

			// Try to parse PCF header
			if len(msgData) >= 36 {
				fmt.Printf("PCF Header Analysis:\n")
				fmt.Printf("  Type (BE):           %d\n", binary.BigEndian.Uint32(msgData[0:4]))
				fmt.Printf("  Type (LE):           %d\n", binary.LittleEndian.Uint32(msgData[0:4]))
				fmt.Printf("  StrucLength (BE):    %d\n", binary.BigEndian.Uint32(msgData[4:8]))
				fmt.Printf("  StrucLength (LE):    %d\n", binary.LittleEndian.Uint32(msgData[4:8]))
				fmt.Printf("  Command (BE):        %d\n", binary.BigEndian.Uint32(msgData[12:16]))
				fmt.Printf("  Command (LE):        %d\n", binary.LittleEndian.Uint32(msgData[12:16]))
				fmt.Printf("  ParamCount (BE):     %d\n", binary.BigEndian.Uint32(msgData[32:36]))
				fmt.Printf("  ParamCount (LE):     %d\n", binary.LittleEndian.Uint32(msgData[32:36]))
			}
		}
	}

	fmt.Println("\n=== Analysis Complete ===")
	fmt.Println("This raw data shows the actual PCF format used by IBM MQ.")
	fmt.Println("Look for ipprocs (input processes/readers) and opprocs (output processes/writers) in the data.")
}
